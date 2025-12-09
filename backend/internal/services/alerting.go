package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/models"
)

// AlertingService моніторить пристрої та генерує алерти
// Алерти логуються і можуть бути підхоплені GCP Cloud Monitoring
type AlertingService struct {
	db              *database.DB
	config          AlertConfig
	deviceLastSeen  map[string]time.Time
	alertsSent      map[string]time.Time
	mu              sync.RWMutex
	stopChan        chan struct{}
}

type AlertConfig struct {
	Enabled           bool
	CheckInterval     time.Duration
	OfflineThreshold  time.Duration
	AlertCooldown     time.Duration
	TempMin           float64
	TempMax           float64
	HumidityMin       float64
	HumidityMax       float64
}

type Alert struct {
	Type        string    `json:"type"`
	DeviceID    string    `json:"device_id"`
	DeviceName  string    `json:"device_name"`
	Message     string    `json:"message"`
	Severity    string    `json:"severity"`
	Timestamp   time.Time `json:"timestamp"`
	Value       float64   `json:"value,omitempty"`
	Threshold   float64   `json:"threshold,omitempty"`
}

func NewAlertingService(db *database.DB, config AlertConfig) *AlertingService {
	if config.CheckInterval == 0 {
		config.CheckInterval = 1 * time.Minute
	}
	if config.OfflineThreshold == 0 {
		config.OfflineThreshold = 5 * time.Minute
	}
	if config.AlertCooldown == 0 {
		config.AlertCooldown = 30 * time.Minute
	}
	
	return &AlertingService{
		db:             db,
		config:         config,
		deviceLastSeen: make(map[string]time.Time),
		alertsSent:     make(map[string]time.Time),
		stopChan:       make(chan struct{}),
	}
}

func (s *AlertingService) Start(ctx context.Context) {
	if !s.config.Enabled {
		log.Println("Alerting service disabled")
		return
	}
	
	log.Println("Starting alerting service...")
	log.Printf("  Check interval: %v", s.config.CheckInterval)
	log.Printf("  Offline threshold: %v", s.config.OfflineThreshold)
	log.Printf("  Temp range: %.1f - %.1f°C", s.config.TempMin, s.config.TempMax)
	log.Printf("  Humidity max: %.1f%%", s.config.HumidityMax)
	
	ticker := time.NewTicker(s.config.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkDevices(ctx)
		}
	}
}

func (s *AlertingService) Stop() {
	close(s.stopChan)
}

func (s *AlertingService) UpdateDeviceLastSeen(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceLastSeen[deviceID] = time.Now()
}

func (s *AlertingService) CheckMetricThresholds(device *models.Device, metric *models.Metric) {
	if !s.config.Enabled {
		return
	}
	
	// Check temperature
	if metric.Temperature != nil {
		temp := *metric.Temperature
		if s.config.TempMax > 0 && temp > s.config.TempMax {
			s.sendAlert(Alert{
				Type:       "temperature_high",
				DeviceID:   device.ID.String(),
				DeviceName: device.Name,
				Message:    fmt.Sprintf("🌡️ High temperature: %.1f°C (threshold: %.1f°C)", temp, s.config.TempMax),
				Severity:   "WARNING",
				Timestamp:  time.Now(),
				Value:      temp,
				Threshold:  s.config.TempMax,
			})
		}
		
		if s.config.TempMin > 0 && temp < s.config.TempMin {
			s.sendAlert(Alert{
				Type:       "temperature_low",
				DeviceID:   device.ID.String(),
				DeviceName: device.Name,
				Message:    fmt.Sprintf("🥶 Low temperature: %.1f°C (threshold: %.1f°C)", temp, s.config.TempMin),
				Severity:   "WARNING",
				Timestamp:  time.Now(),
				Value:      temp,
				Threshold:  s.config.TempMin,
			})
		}
	}
	
	// Check humidity
	if metric.Humidity != nil {
		humidity := *metric.Humidity
		if s.config.HumidityMax > 0 && humidity > s.config.HumidityMax {
			s.sendAlert(Alert{
				Type:       "humidity_high",
				DeviceID:   device.ID.String(),
				DeviceName: device.Name,
				Message:    fmt.Sprintf("💧 High humidity: %.1f%% (threshold: %.1f%%)", humidity, s.config.HumidityMax),
				Severity:   "WARNING",
				Timestamp:  time.Now(),
				Value:      humidity,
				Threshold:  s.config.HumidityMax,
			})
		}
	}
}

func (s *AlertingService) checkDevices(ctx context.Context) {
	devices, err := s.db.GetAllDevices(ctx)
	if err != nil {
		log.Printf("Alerting: failed to get devices: %v", err)
		return
	}
	
	now := time.Now()
	
	for _, device := range devices {
		deviceIDStr := device.ID.String()
		
		s.mu.RLock()
		lastSeen, exists := s.deviceLastSeen[deviceIDStr]
		s.mu.RUnlock()
		
		if !exists && device.LastSeen != nil {
			lastSeen = *device.LastSeen
			s.mu.Lock()
			s.deviceLastSeen[deviceIDStr] = lastSeen
			s.mu.Unlock()
			exists = true
		}
		
		// Device went offline
		if device.IsOnline && exists && now.Sub(lastSeen) > s.config.OfflineThreshold {
			s.sendAlert(Alert{
				Type:       "device_offline",
				DeviceID:   deviceIDStr,
				DeviceName: device.Name,
				Message:    fmt.Sprintf("🔴 Device '%s' went OFFLINE! Last seen: %s", device.Name, lastSeen.Format("15:04:05")),
				Severity:   "CRITICAL",
				Timestamp:  now,
			})
			
			if err := s.db.UpdateDeviceOnline(ctx, device.ID, false); err != nil {
				log.Printf("Failed to update device status: %v", err)
			}
		}
		
		// Device came back online
		if !device.IsOnline && exists && now.Sub(lastSeen) < s.config.OfflineThreshold {
			s.sendAlert(Alert{
				Type:       "device_online",
				DeviceID:   deviceIDStr,
				DeviceName: device.Name,
				Message:    fmt.Sprintf("🟢 Device '%s' is back ONLINE!", device.Name),
				Severity:   "INFO",
				Timestamp:  now,
			})
		}
	}
}

func (s *AlertingService) sendAlert(alert Alert) {
	// Check cooldown
	alertKey := fmt.Sprintf("%s:%s", alert.Type, alert.DeviceID)
	
	s.mu.RLock()
	lastAlert, exists := s.alertsSent[alertKey]
	s.mu.RUnlock()
	
	if exists && time.Since(lastAlert) < s.config.AlertCooldown {
		return
	}
	
	s.mu.Lock()
	s.alertsSent[alertKey] = time.Now()
	s.mu.Unlock()
	
	// Log in structured format for GCP Cloud Logging
	// GCP Cloud Monitoring can create alerts based on these log entries
	log.Printf("ALERT [%s] device=%s name=%s msg=%s value=%.2f threshold=%.2f",
		alert.Severity,
		alert.DeviceID,
		alert.DeviceName,
		alert.Message,
		alert.Value,
		alert.Threshold,
	)
}
