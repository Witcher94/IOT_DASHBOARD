package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/models"
)

type DeviceService struct {
	db *database.DB
}

func NewDeviceService(db *database.DB) *DeviceService {
	return &DeviceService{db: db}
}

func (s *DeviceService) GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *DeviceService) CreateDevice(ctx context.Context, userID uuid.UUID, name string, deviceType string) (*models.Device, error) {
	token, err := s.GenerateToken()
	if err != nil {
		return nil, err
	}

	if deviceType == "" {
		deviceType = models.DeviceTypeSimple
	}

	device := &models.Device{
		UserID:      userID,
		Name:        name,
		Token:       token,
		DeviceType:  deviceType,
		DHTEnabled:  true,
		MeshEnabled: deviceType == models.DeviceTypeGateway, // Gateway has mesh enabled
	}

	if err := s.db.CreateDevice(ctx, device); err != nil {
		return nil, err
	}

	return device, nil
}

func (s *DeviceService) GetUserDevices(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	return s.db.GetDevicesByUserID(ctx, userID)
}

func (s *DeviceService) GetDevice(ctx context.Context, deviceID uuid.UUID) (*models.Device, error) {
	return s.db.GetDeviceByID(ctx, deviceID)
}

func (s *DeviceService) DeleteDevice(ctx context.Context, deviceID uuid.UUID) error {
	return s.db.DeleteDevice(ctx, deviceID)
}

func (s *DeviceService) UpdateDeviceFromMetrics(ctx context.Context, device *models.Device, payload *models.DeviceMetricsPayload) error {
	now := time.Now()

	device.ChipID = &payload.System.ChipID
	device.MAC = &payload.System.MAC
	device.Platform = &payload.System.Platform
	device.Firmware = &payload.System.Firmware
	device.IsOnline = true
	device.LastSeen = &now
	device.DHTEnabled = payload.DHTEnabled
	device.MeshEnabled = payload.MeshStatus.Enabled

	return s.db.UpdateDevice(ctx, device)
}

func (s *DeviceService) ProcessMetrics(ctx context.Context, device *models.Device, payload *models.DeviceMetricsPayload) error {
	// Update device info
	if err := s.UpdateDeviceFromMetrics(ctx, device, payload); err != nil {
		return err
	}

	// Create metric record
	var rssi *int
	if payload.CurrentWifi != nil {
		rssi = &payload.CurrentWifi.RSSI
	}

	metric := &models.Metric{
		DeviceID:    device.ID,
		Temperature: payload.Temperature,
		Humidity:    payload.Humidity,
		RSSI:        rssi,
		FreeHeap:    &payload.System.FreeHeap,
		WifiScan:    payload.WifiScan,
		MeshNodes:   payload.MeshNeighbors,
	}

	return s.db.CreateMetric(ctx, metric)
}

func (s *DeviceService) GetMetrics(ctx context.Context, deviceID uuid.UUID, limit int) ([]models.Metric, error) {
	return s.db.GetMetricsByDeviceID(ctx, deviceID, limit)
}

func (s *DeviceService) GetMetricsForPeriod(ctx context.Context, deviceID uuid.UUID, start, end time.Time) ([]models.Metric, error) {
	return s.db.GetMetricsForPeriod(ctx, deviceID, start, end)
}

func (s *DeviceService) CreateCommand(ctx context.Context, deviceID uuid.UUID, req *models.CreateCommandRequest) (*models.Command, error) {
	params, _ := json.Marshal(map[string]interface{}{
		"firmware_url": req.FirmwareURL,
		"interval":     req.Interval,
		"name":         req.Name,
	})

	cmd := &models.Command{
		DeviceID: deviceID,
		Command:  req.Command,
		Params:   string(params),
		Status:   "pending",
	}

	if err := s.db.CreateCommand(ctx, cmd); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (s *DeviceService) GetPendingCommand(ctx context.Context, deviceID uuid.UUID) (*models.DeviceCommand, error) {
	cmd, err := s.db.GetPendingCommand(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	// Mark as sent
	s.db.MarkCommandSent(ctx, cmd.ID)

	// Parse params
	var params map[string]interface{}
	json.Unmarshal([]byte(cmd.Params), &params)

	deviceCmd := &models.DeviceCommand{
		ID:      cmd.ID.String(),
		Command: cmd.Command,
	}

	if url, ok := params["firmware_url"].(string); ok {
		deviceCmd.FirmwareURL = url
	}
	if interval, ok := params["interval"].(float64); ok {
		deviceCmd.Interval = int(interval)
	}
	if name, ok := params["name"].(string); ok {
		deviceCmd.Name = name
	}

	return deviceCmd, nil
}

func (s *DeviceService) AcknowledgeCommand(ctx context.Context, commandID uuid.UUID, status string) error {
	return s.db.AcknowledgeCommand(ctx, commandID, status)
}

func (s *DeviceService) RegenerateToken(ctx context.Context, deviceID uuid.UUID) (string, error) {
	device, err := s.db.GetDeviceByID(ctx, deviceID)
	if err != nil {
		return "", err
	}

	newToken, err := s.GenerateToken()
	if err != nil {
		return "", err
	}

	device.Token = newToken
	if err := s.db.UpdateDevice(ctx, device); err != nil {
		return "", err
	}

	return newToken, nil
}

