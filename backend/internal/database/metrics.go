package database

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/models"
)

func (db *DB) CreateMetric(ctx context.Context, metric *models.Metric) error {
	wifiScanJSON, _ := json.Marshal(metric.WifiScan)
	meshNodesJSON, _ := json.Marshal(metric.MeshNodes)

	query := `
		INSERT INTO metrics (device_id, temperature, humidity, rssi, free_heap, wifi_scan, mesh_nodes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	return db.Pool.QueryRow(ctx, query,
		metric.DeviceID, metric.Temperature, metric.Humidity, metric.RSSI,
		metric.FreeHeap, wifiScanJSON, meshNodesJSON,
	).Scan(&metric.ID, &metric.CreatedAt)
}

func (db *DB) GetMetricsByDeviceID(ctx context.Context, deviceID uuid.UUID, limit int) ([]models.Metric, error) {
	query := `
		SELECT id, device_id, temperature, humidity, rssi, free_heap, wifi_scan, mesh_nodes, created_at
		FROM metrics WHERE device_id = $1
		ORDER BY created_at DESC LIMIT $2
	`
	rows, err := db.Pool.Query(ctx, query, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.Metric
	for rows.Next() {
		var metric models.Metric
		var wifiScanJSON, meshNodesJSON []byte

		if err := rows.Scan(
			&metric.ID, &metric.DeviceID, &metric.Temperature, &metric.Humidity,
			&metric.RSSI, &metric.FreeHeap, &wifiScanJSON, &meshNodesJSON, &metric.CreatedAt,
		); err != nil {
			return nil, err
		}

		json.Unmarshal(wifiScanJSON, &metric.WifiScan)
		json.Unmarshal(meshNodesJSON, &metric.MeshNodes)

		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func (db *DB) GetLatestMetricByDeviceID(ctx context.Context, deviceID uuid.UUID) (*models.Metric, error) {
	query := `
		SELECT id, device_id, temperature, humidity, rssi, free_heap, wifi_scan, mesh_nodes, created_at
		FROM metrics WHERE device_id = $1
		ORDER BY created_at DESC LIMIT 1
	`
	metric := &models.Metric{}
	var wifiScanJSON, meshNodesJSON []byte

	err := db.Pool.QueryRow(ctx, query, deviceID).Scan(
		&metric.ID, &metric.DeviceID, &metric.Temperature, &metric.Humidity,
		&metric.RSSI, &metric.FreeHeap, &wifiScanJSON, &meshNodesJSON, &metric.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	json.Unmarshal(wifiScanJSON, &metric.WifiScan)
	json.Unmarshal(meshNodesJSON, &metric.MeshNodes)

	return metric, nil
}

func (db *DB) GetMetricsForPeriod(ctx context.Context, deviceID uuid.UUID, start, end time.Time) ([]models.Metric, error) {
	query := `
		SELECT id, device_id, temperature, humidity, rssi, free_heap, wifi_scan, mesh_nodes, created_at
		FROM metrics WHERE device_id = $1 AND created_at >= $2 AND created_at <= $3
		ORDER BY created_at ASC
	`
	rows, err := db.Pool.Query(ctx, query, deviceID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.Metric
	for rows.Next() {
		var metric models.Metric
		var wifiScanJSON, meshNodesJSON []byte

		if err := rows.Scan(
			&metric.ID, &metric.DeviceID, &metric.Temperature, &metric.Humidity,
			&metric.RSSI, &metric.FreeHeap, &wifiScanJSON, &meshNodesJSON, &metric.CreatedAt,
		); err != nil {
			return nil, err
		}

		json.Unmarshal(wifiScanJSON, &metric.WifiScan)
		json.Unmarshal(meshNodesJSON, &metric.MeshNodes)

		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func (db *DB) GetAvgTemperature(ctx context.Context) (float64, error) {
	var avg float64
	query := `
		SELECT COALESCE(AVG(temperature), 0) FROM metrics
		WHERE temperature IS NOT NULL
		AND created_at > NOW() - INTERVAL '1 hour'
	`
	err := db.Pool.QueryRow(ctx, query).Scan(&avg)
	return avg, err
}

func (db *DB) GetAvgHumidity(ctx context.Context) (float64, error) {
	var avg float64
	query := `
		SELECT COALESCE(AVG(humidity), 0) FROM metrics
		WHERE humidity IS NOT NULL
		AND created_at > NOW() - INTERVAL '1 hour'
	`
	err := db.Pool.QueryRow(ctx, query).Scan(&avg)
	return avg, err
}

func (db *DB) DeleteOldMetrics(ctx context.Context, olderThan time.Duration) error {
	query := `DELETE FROM metrics WHERE created_at < $1`
	_, err := db.Pool.Exec(ctx, query, time.Now().Add(-olderThan))
	return err
}

