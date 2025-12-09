# 🔔 Налаштування алертів IoT Dashboard

## Як це працює

```
┌─────────────┐     ┌──────────────┐     ┌─────────────────┐
│   ESP32     │────▶│   Backend    │────▶│  Cloud Logging  │
│   Metrics   │     │   Alerting   │     │                 │
└─────────────┘     └──────────────┘     └────────┬────────┘
                                                   │
                    ┌──────────────────────────────▼────────────────────┐
                    │              GCP Cloud Monitoring                 │
                    │  ┌─────────────────┐  ┌─────────────────┐        │
                    │  │  Alert Policy   │  │  Alert Policy   │        │
                    │  │  Device Offline │  │  High Temp      │        │
                    │  └────────┬────────┘  └────────┬────────┘        │
                    │           │                     │                 │
                    │           ▼                     ▼                 │
                    │  ┌─────────────────────────────────────────┐     │
                    │  │       Notification Channels             │     │
                    │  │   📧 Email    📱 SMS    💬 Slack/etc    │     │
                    │  └─────────────────────────────────────────┘     │
                    └───────────────────────────────────────────────────┘
```

---

## 🚀 Крок 1: Налаштування в UI

Пороги для кожного пристрою налаштовуються на сторінці пристрою:

```
PUT /api/v1/devices/{id}/alerts

{
  "alerts_enabled": true,
  "alert_temp_min": 5,      // Алерт якщо < 5°C
  "alert_temp_max": 35,     // Алерт якщо > 35°C
  "alert_humidity_max": 80  // Алерт якщо > 80%
}
```

---

## ☁️ Крок 2: Налаштування GCP Cloud Monitoring

### 2.1 Створити Notification Channel

**Email (безкоштовно):**

```bash
gcloud beta monitoring channels create \
  --type=email \
  --display-name="IoT Alerts" \
  --channel-labels=email_address=your-email@gmail.com
```

**SMS (потребує верифікації номера):**

```bash
gcloud beta monitoring channels create \
  --type=sms \
  --display-name="IoT SMS Alerts" \
  --channel-labels=number=+380XXXXXXXXX
```

Збережіть ID каналу:
```bash
export CHANNEL_ID=$(gcloud beta monitoring channels list \
  --format='value(name)' \
  --filter='displayName="IoT Alerts"')
```

### 2.2 Створити Alert Policy - Device Offline

```bash
gcloud alpha monitoring policies create \
  --display-name="IoT Device Offline" \
  --notification-channels=$CHANNEL_ID \
  --condition-display-name="No heartbeat for 5 min" \
  --condition-filter='resource.type="cloud_run_revision" AND textPayload=~"device_offline"' \
  --aggregation='{"alignmentPeriod": "60s", "perSeriesAligner": "ALIGN_COUNT"}' \
  --condition-threshold-comparison=COMPARISON_GT \
  --condition-threshold-value=0 \
  --duration=0s
```

### 2.3 Створити Alert Policy - High Temperature

```bash
gcloud alpha monitoring policies create \
  --display-name="IoT High Temperature" \
  --notification-channels=$CHANNEL_ID \
  --condition-display-name="Temperature alert triggered" \
  --condition-filter='resource.type="cloud_run_revision" AND textPayload=~"temperature_high"' \
  --aggregation='{"alignmentPeriod": "60s", "perSeriesAligner": "ALIGN_COUNT"}' \
  --condition-threshold-comparison=COMPARISON_GT \
  --condition-threshold-value=0 \
  --duration=0s
```

---

## 🔧 Крок 3: Terraform конфігурація

Додайте до `terraform/main.tf`:

```hcl
# Notification Channel - Email
resource "google_monitoring_notification_channel" "email" {
  display_name = "IoT Dashboard Alerts"
  type         = "email"
  
  labels = {
    email_address = var.admin_email
  }
}

# Alert - Device Offline
resource "google_monitoring_alert_policy" "device_offline" {
  display_name = "IoT Device Offline"
  combiner     = "OR"
  
  conditions {
    display_name = "Device stopped sending metrics"
    
    condition_matched_log {
      filter = 'resource.type="cloud_run_revision" AND textPayload=~"ALERT \\[CRITICAL\\].*device_offline"'
    }
  }
  
  notification_channels = [google_monitoring_notification_channel.email.id]
  
  alert_strategy {
    auto_close = "1800s"
  }
  
  documentation {
    content   = "An IoT device has stopped sending metrics for more than 5 minutes."
    mime_type = "text/markdown"
  }
}

# Alert - High Temperature  
resource "google_monitoring_alert_policy" "high_temperature" {
  display_name = "IoT High Temperature"
  combiner     = "OR"
  
  conditions {
    display_name = "Temperature exceeded threshold"
    
    condition_matched_log {
      filter = 'resource.type="cloud_run_revision" AND textPayload=~"ALERT \\[WARNING\\].*temperature_high"'
    }
  }
  
  notification_channels = [google_monitoring_notification_channel.email.id]
  
  alert_strategy {
    auto_close = "3600s"
  }
}

# Alert - High Humidity
resource "google_monitoring_alert_policy" "high_humidity" {
  display_name = "IoT High Humidity"
  combiner     = "OR"
  
  conditions {
    display_name = "Humidity exceeded threshold"
    
    condition_matched_log {
      filter = 'resource.type="cloud_run_revision" AND textPayload=~"ALERT \\[WARNING\\].*humidity_high"'
    }
  }
  
  notification_channels = [google_monitoring_notification_channel.email.id]
}
```

---

## 📊 Перегляд алертів

1. **GCP Console:** https://console.cloud.google.com/monitoring/alerting
2. **Логи алертів:** https://console.cloud.google.com/logs (фільтр: `ALERT`)

---

## ⚙️ Змінні оточення

```env
# Увімкнути локальний алертинг (логи для Cloud Monitoring)
ALERTING_ENABLED=true

# Перевірка статусу пристроїв кожні:
ALERT_CHECK_INTERVAL=1m

# Пристрій вважається offline після:
ALERT_OFFLINE_THRESHOLD=5m

# Не надсилати повторний алерт протягом:
ALERT_COOLDOWN=30m

# Глобальні пороги (перевизначаються в UI для кожного пристрою)
TEMP_MIN=0
TEMP_MAX=40
HUMIDITY_MAX=90
```

---

## 📱 Налаштування SMS

GCP Cloud Monitoring підтримує SMS нативно:

1. Перейдіть: https://console.cloud.google.com/monitoring/alerting/notifications
2. Натисніть "Edit notification channels"
3. В секції "SMS" натисніть "Add new"
4. Введіть номер телефону (формат: +380XXXXXXXXX)
5. Підтвердіть код з SMS
6. Додайте канал до Alert Policy

**Безкоштовно:** До 50 SMS/місяць на проект.

---

## 💰 Вартість

| Сервіс | Безкоштовно | Після ліміту |
|--------|-------------|--------------|
| Cloud Monitoring | Необмежено | - |
| Email алерти | Необмежено | - |
| SMS алерти | 50/місяць | $0.05/SMS |
| Cloud Logging | 50 GB/місяць | $0.50/GB |

**Для типового IoT проекту: $0/місяць**
