package models

import (
	"time"

	"github.com/google/uuid"
)

// User представляє користувача системи
type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Picture   string    `json:"picture,omitempty"`
	GoogleID  string    `json:"-"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Device представляє IoT пристрій
type Device struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Name         string     `json:"name"`
	Token        string     `json:"token,omitempty"`
	ChipID       string     `json:"chip_id,omitempty"`
	MAC          string     `json:"mac,omitempty"`
	Platform     string     `json:"platform,omitempty"`
	Firmware     string     `json:"firmware,omitempty"`
	IsOnline     bool       `json:"is_online"`
	LastSeen     *time.Time `json:"last_seen,omitempty"`
	DHTEnabled   bool       `json:"dht_enabled"`
	MeshEnabled  bool       `json:"mesh_enabled"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Metric представляє метрики з пристрою
type Metric struct {
	ID          uuid.UUID       `json:"id"`
	DeviceID    uuid.UUID       `json:"device_id"`
	Temperature *float64        `json:"temperature,omitempty"`
	Humidity    *float64        `json:"humidity,omitempty"`
	RSSI        *int            `json:"rssi,omitempty"`
	FreeHeap    *int64          `json:"free_heap,omitempty"`
	Uptime      *int64          `json:"uptime,omitempty"`
	WifiScan    []WifiNetwork   `json:"wifi_scan,omitempty"`
	MeshNodes   []MeshNeighbor  `json:"mesh_neighbors,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
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
	Enabled   bool `json:"enabled"`
	Running   bool `json:"running"`
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
	ID          uuid.UUID  `json:"id"`
	DeviceID    uuid.UUID  `json:"device_id"`
	Command     string     `json:"command"`
	Params      string     `json:"params,omitempty"`
	Status      string     `json:"status"` // pending, sent, acknowledged, failed
	CreatedAt   time.Time  `json:"created_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	AckedAt     *time.Time `json:"acked_at,omitempty"`
}

// DeviceCommand структура для відправки на ESP
type DeviceCommand struct {
	ID          string `json:"id"`
	Command     string `json:"command"`
	FirmwareURL string `json:"firmware_url,omitempty"`
	Interval    int    `json:"interval,omitempty"`
	Name        string `json:"name,omitempty"`
}

// CreateDeviceRequest запит на створення пристрою
type CreateDeviceRequest struct {
	Name string `json:"name" binding:"required"`
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

