package models

import (
	"time"

	"github.com/google/uuid"
)

// User представляє користувача системи
type User struct {
	ID                    uuid.UUID `json:"id"`
	Email                 string    `json:"email"`
	Name                  string    `json:"name"`
	Picture               string    `json:"picture,omitempty"`
	GoogleID              string    `json:"-"`
	IsAdmin               bool      `json:"is_admin"`
	NotificationChannelID string    `json:"-"` // GCP Monitoring channel
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// DeviceType types
const (
	DeviceTypeSimple   = "simple_device"
	DeviceTypeGateway  = "gateway"
	DeviceTypeMeshNode = "mesh_node"
	DeviceTypeSKUD     = "skud" // Access control device with challenge-response auth
)

// Device представляє IoT пристрій
type Device struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	Name        string     `json:"name"`
	Token       string     `json:"-"`                      // Hidden by default, use DeviceWithToken for responses that need it
	DeviceType  string     `json:"device_type"`            // simple_device, gateway, mesh_node
	GatewayID   *uuid.UUID `json:"gateway_id,omitempty"`   // Parent gateway for mesh_node
	MeshNodeID  *uint32    `json:"mesh_node_id,omitempty"` // painlessMesh node ID
	ChipID      *string    `json:"chip_id,omitempty"`
	MAC         *string    `json:"mac,omitempty"`
	Platform    *string    `json:"platform,omitempty"`
	Firmware    *string    `json:"firmware,omitempty"`
	IsOnline    bool       `json:"is_online"`
	LastSeen    *time.Time `json:"last_seen,omitempty"`
	DHTEnabled  bool       `json:"dht_enabled"`
	MeshEnabled bool       `json:"mesh_enabled"`
	// Alert settings
	AlertsEnabled    bool      `json:"alerts_enabled"`
	AlertTempMin     *float64  `json:"alert_temp_min,omitempty"`
	AlertTempMax     *float64  `json:"alert_temp_max,omitempty"`
	AlertHumidityMax *float64  `json:"alert_humidity_max,omitempty"`
	AlertPolicyID    string    `json:"-"` // Internal, not exposed to API
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Nested mesh nodes (only populated for gateways)
	MeshNodes []Device `json:"mesh_nodes,omitempty"`
}

// DeviceWithToken - Device response that includes the token (for create/regenerate)
type DeviceWithToken struct {
	Device
	Token string `json:"token"`
}

// Metric представляє метрики з пристрою
type Metric struct {
	ID          uuid.UUID      `json:"id"`
	DeviceID    uuid.UUID      `json:"device_id"`
	Temperature *float64       `json:"temperature,omitempty"`
	Humidity    *float64       `json:"humidity,omitempty"`
	RSSI        *int           `json:"rssi,omitempty"`
	FreeHeap    *int64         `json:"free_heap,omitempty"`
	Uptime      *int64         `json:"uptime,omitempty"`
	WifiScan    []WifiNetwork  `json:"wifi_scan,omitempty"`
	MeshNodes   []MeshNeighbor `json:"mesh_neighbors,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// WifiNetwork представляє знайдену WiFi мережу
type WifiNetwork struct {
	SSID    string `json:"ssid"`
	RSSI    int    `json:"rssi"`
	BSSID   string `json:"bssid"`
	Channel int    `json:"channel"`
	Enc     string `json:"enc"`
}

// MeshNeighbor представляє сусіда в mesh мережі
type MeshNeighbor struct {
	ID uint32 `json:"id"`
}

// MeshStatus статус mesh мережі
type MeshStatus struct {
	Enabled   bool   `json:"enabled"`
	Running   bool   `json:"running"`
	NodeID    uint32 `json:"node_id"`
	NodeCount int    `json:"node_count"`
}

// CurrentWifi поточне WiFi з'єднання
type CurrentWifi struct {
	SSID    string `json:"ssid"`
	RSSI    int    `json:"rssi"`
	BSSID   string `json:"bssid"`
	IP      string `json:"ip"`
	Channel int    `json:"channel"`
}

// SystemInfo інформація про систему ESP
type SystemInfo struct {
	ChipID   string `json:"chip_id"`
	MAC      string `json:"mac"`
	Firmware string `json:"firmware"`
	Platform string `json:"platform"`
	FreeHeap int64  `json:"free_heap"`
	CPUFreq  int    `json:"cpu_freq"`
}

// DeviceMetricsPayload повний payload з ESP
type DeviceMetricsPayload struct {
	NodeName      string         `json:"node_name"`
	NodeID        uint32         `json:"node_id"`
	DeviceToken   string         `json:"device_token"`
	Temperature   *float64       `json:"temperature"`
	Humidity      *float64       `json:"humidity"`
	DHTEnabled    bool           `json:"dht_enabled"`
	System        SystemInfo     `json:"system"`
	CurrentWifi   *CurrentWifi   `json:"current_wifi"`
	WifiScan      []WifiNetwork  `json:"wifi_scan"`
	MeshNeighbors []MeshNeighbor `json:"mesh_neighbors"`
	MeshStatus    MeshStatus     `json:"mesh_status"`
}

// Command команда для пристрою
type Command struct {
	ID        uuid.UUID  `json:"id"`
	DeviceID  uuid.UUID  `json:"device_id"`
	Command   string     `json:"command"`
	Params    string     `json:"params,omitempty"`
	Status    string     `json:"status"` // pending, sent, acknowledged, failed
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
	AckedAt   *time.Time `json:"acked_at,omitempty"`
}

// DeviceCommand структура для відправки на ESP
type DeviceCommand struct {
	ID          string `json:"id"`
	Command     string `json:"command"`
	FirmwareURL string `json:"firmware_url,omitempty"`
	Interval    int    `json:"interval,omitempty"`
	Name        string `json:"name,omitempty"`
}

// DeviceShare represents shared access to a device
type DeviceShare struct {
	ID           uuid.UUID `json:"id"`
	DeviceID     uuid.UUID `json:"device_id"`
	OwnerID      uuid.UUID `json:"owner_id"`
	SharedWithID uuid.UUID `json:"shared_with_id"`
	Permission   string    `json:"permission"` // "view" or "edit"
	CreatedAt    time.Time `json:"created_at"`
	// Joined fields for display
	SharedWithName  string `json:"shared_with_name,omitempty"`
	SharedWithEmail string `json:"shared_with_email,omitempty"`
}

// ShareDeviceRequest request to share a device
type ShareDeviceRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Permission string `json:"permission"` // "view" or "edit" (default: view)
}

// CreateDeviceRequest запит на створення пристрою
type CreateDeviceRequest struct {
	Name       string `json:"name" binding:"required"`
	DeviceType string `json:"device_type"` // simple_device, gateway (default: simple_device)
}

// BatchMetricsPayload payload від gateway з метриками всіх нод
type BatchMetricsPayload struct {
	GatewayID      string             `json:"gateway_id"`
	Timestamp      time.Time          `json:"timestamp"`
	Nodes          []NodeMetricsBatch `json:"nodes"`
	GatewayMetrics *GatewayMetrics    `json:"gateway_metrics,omitempty"`
}

// GatewayMetrics represents RPi gateway system metrics
type GatewayMetrics struct {
	CPUUsage    float64 `json:"cpu_usage"`    // CPU usage %
	MemoryUsage float64 `json:"memory_usage"` // Memory usage %
	CPUTemp     float64 `json:"cpu_temp"`     // CPU temperature in Celsius
	Uptime      int64   `json:"uptime"`       // Uptime in seconds
}

// NodeMetricsBatch метрики однієї ноди в batch
type NodeMetricsBatch struct {
	NodeID      uint32  `json:"node_id"` // painlessMesh node ID
	NodeName    string  `json:"node_name"`
	ChipID      string  `json:"chip_id"`
	MAC         string  `json:"mac"`
	Platform    string  `json:"platform"`
	Firmware    string  `json:"firmware"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	FreeHeap    int64   `json:"free_heap"`
	RSSI        int     `json:"rssi"`
	DHTEnabled  bool    `json:"dht_enabled"`
}

// GatewayTopology структура для відображення топології
type GatewayTopology struct {
	Gateway     Device   `json:"gateway"`
	MeshNodes   []Device `json:"mesh_nodes"`
	TotalNodes  int      `json:"total_nodes"`
	OnlineNodes int      `json:"online_nodes"`
}

// UpdateAlertSettingsRequest запит на оновлення налаштувань алертів
type UpdateAlertSettingsRequest struct {
	AlertsEnabled    *bool    `json:"alerts_enabled"`
	AlertTempMin     *float64 `json:"alert_temp_min"`
	AlertTempMax     *float64 `json:"alert_temp_max"`
	AlertHumidityMax *float64 `json:"alert_humidity_max"`
}

// CreateCommandRequest запит на створення команди
type CreateCommandRequest struct {
	Command     string `json:"command" binding:"required"`
	FirmwareURL string `json:"firmware_url,omitempty"`
	Interval    int    `json:"interval,omitempty"`
	Name        string `json:"name,omitempty"`
}

// LoginResponse відповідь після логіну
type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// DashboardStats статистика для дашборду
type DashboardStats struct {
	TotalDevices  int64   `json:"total_devices"`
	OnlineDevices int64   `json:"online_devices"`
	TotalUsers    int64   `json:"total_users"`
	AvgTemp       float64 `json:"avg_temperature"`
	AvgHumidity   float64 `json:"avg_humidity"`
}

// ==================== SKUD (Access Control) Models ====================

// CardStatus types
const (
	CardStatusPending  = "pending"
	CardStatusActive   = "active"
	CardStatusDisabled = "disabled"
)

// Card представляє NFC/RFID картку доступу
type Card struct {
	ID        uuid.UUID     `json:"id"`
	CardUID   string        `json:"card_uid"`
	CardType  string        `json:"card_type"` // MIFARE_CLASSIC_1K, MIFARE_DESFIRE, etc.
	Name      string        `json:"name"`      // Custom display name for the card
	Status    string        `json:"status"`    // pending, active, disabled
	Devices   []DeviceBrief `json:"devices,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// DeviceBrief - короткий опис пристрою для зв'язку з карткою
type DeviceBrief struct {
	ID       uuid.UUID `json:"id"`
	DeviceID string    `json:"device_id"` // hardware device_id
	Name     string    `json:"name"`
}

// AccessDevice представляє ESP пристрій СКУД з секретним ключем
type AccessDevice struct {
	ID        uuid.UUID `json:"id"`
	DeviceID  string    `json:"device_id"`  // hardware identifier
	SecretKey string    `json:"secret_key"` // API key for device authentication
	Name      string    `json:"name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AccessVerifyRequest запит на верифікацію доступу (з challenge-response для SKUD)
type AccessVerifyRequest struct {
	CardUID   string `json:"card_uid" binding:"required"`
	CardType  string `json:"card_type"`
	Challenge string `json:"challenge"` // Required for SKUD devices (challenge-response)
}

// AccessVerifyResponse відповідь на верифікацію
type AccessVerifyResponse struct {
	Access   bool   `json:"access"`
	CardName string `json:"card_name,omitempty"` // Display name for ESP screen
}

// AccessRegisterRequest запит на реєстрацію нової картки (з challenge-response для SKUD)
type AccessRegisterRequest struct {
	CardUID   string `json:"card_uid" binding:"required"`
	CardType  string `json:"card_type"`
	Challenge string `json:"challenge"` // Required for SKUD devices (challenge-response)
}

// ChallengeResponse відповідь з challenge для SKUD пристроїв
type ChallengeResponse struct {
	Challenge string `json:"challenge"`
	ExpiresIn int    `json:"expires_in"` // seconds until expiration
}

// AccessRegisterResponse відповідь на реєстрацію
type AccessRegisterResponse struct {
	Status string `json:"status"`
}

// UpdateCardStatusRequest запит на оновлення статусу картки
type UpdateCardStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateCardRequest запит на оновлення картки (name та інші поля)
type UpdateCardRequest struct {
	Name   *string `json:"name"`
	Status *string `json:"status"`
}

// CreateAccessDeviceRequest запит на створення пристрою СКУД
type CreateAccessDeviceRequest struct {
	DeviceID  string `json:"device_id" binding:"required"`
	SecretKey string `json:"secret_key" binding:"required"`
	Name      string `json:"name"`
}

// AccessLog запис логу доступу
type AccessLog struct {
	ID        uuid.UUID `json:"id"`
	DeviceID  string    `json:"device_id"`
	CardUID   string    `json:"card_uid"`
	CardType  string    `json:"card_type"`
	Action    string    `json:"action"` // verify, register, card_status, card_delete
	Status    string    `json:"status"`
	Allowed   bool      `json:"allowed"`
	CreatedAt time.Time `json:"created_at"`
}
