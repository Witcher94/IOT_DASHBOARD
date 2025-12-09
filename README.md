# 🌐 IoT Dashboard - ESP32/ESP8266 Mesh Network Monitor

Повноцінна система моніторингу IoT пристроїв на базі ESP32/ESP8266 з mesh мережею, датчиками DHT22, OTA оновленнями та красивим веб-інтерфейсом.

![Dashboard Preview](docs/dashboard-preview.png)

## ✨ Функціонал

### 🔧 ESP Прошивка
- **Mesh мережа** - painlessMesh для зв'язку між пристроями
- **WiFi сканування** - список доступних мереж з рівнем сигналу
- **DHT22 датчик** - температура та вологість
- **OTA оновлення** - віддалене оновлення прошивки
- **Токен авторизація** - безпечне підключення до бекенду
- **Веб-конфігурація** - налаштування через браузер

### 🖥️ Backend (Go)
- **REST API** - повний CRUD для пристроїв та метрик
- **Google OAuth2** - авторизація через Google акаунт
- **WebSocket** - real-time оновлення даних
- **JWT токени** - безпечна авторизація
- **PostgreSQL** - надійне зберігання даних
- **Команди для ESP** - reboot, toggle sensors, OTA та інше

### 🎨 Frontend (React)
- **Сучасний дизайн** - темна тема з градієнтами
- **Real-time дані** - WebSocket оновлення
- **Графіки** - Chart.js візуалізація метрик
- **Адаптивність** - працює на всіх пристроях
- **Адмін панель** - управління користувачами та пристроями

## 🚀 Швидкий старт

### Вимоги
- Docker & Docker Compose
- Node.js 18+ (для локальної розробки)
- Go 1.21+ (для локальної розробки)
- Google Cloud Console проект (для OAuth)

### 1. Клонування та налаштування

```bash
# Клонуй репозиторій
git clone https://github.com/your-repo/iot-dashboard.git
cd iot-dashboard

# Створи .env файл
cp backend/.env.example .env

# Відредагуй .env та додай свої credentials
nano .env
```

### 2. Google OAuth Setup

1. Перейди на [Google Cloud Console](https://console.cloud.google.com/)
2. Створи новий проект або вибери існуючий
3. APIs & Services → Credentials → Create Credentials → OAuth 2.0 Client ID
4. Application type: Web application
5. Authorized redirect URIs: `http://localhost/api/v1/auth/google/callback`
6. Скопіюй Client ID та Client Secret в `.env`

### 3. Запуск з Docker

```bash
# Запуск всіх сервісів
docker-compose up -d

# Перевірка логів
docker-compose logs -f

# Зупинка
docker-compose down
```

Відкрий http://localhost в браузері.

### 4. Локальна розробка

```bash
# Запуск тільки бази даних
docker-compose -f docker-compose.dev.yml up -d

# Backend
cd backend
go mod download
go run cmd/server/main.go

# Frontend (в іншому терміналі)
cd frontend
npm install
npm run dev
```

## 📱 Налаштування ESP

### 1. Підготовка

Встанови необхідні бібліотеки в Arduino IDE:
- painlessMesh
- ArduinoJson
- DHT sensor library
- HTTPClient (ESP32) / ESP8266HTTPClient

### 2. Прошивка

1. Відкрий `esp32_esp8266.ino` в Arduino IDE
2. Вибери правильну плату (ESP32 або ESP8266)
3. Завантаж прошивку на пристрій

### 3. Конфігурація

1. ESP створить WiFi точку доступу "ESP-IOT-CONFIG"
2. Підключись до неї з телефону/комп'ютера
3. Відкрий http://192.168.4.1 в браузері
4. Введи:
   - WiFi мережу та пароль
   - Назву пристрою
   - Backend URL (наприклад: `http://your-server.com`)
   - Device Token (скопіюй з дашборду)
5. Збережи та перезавантаж

### 4. Отримання токена

1. Увійди в дашборд через Google
2. Перейди в Devices → Add Device
3. Введи назву пристрою
4. Скопіюй згенерований токен
5. Встав токен в конфігурацію ESP

## 🏗️ Архітектура

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   ESP32/8266    │────▶│   Go Backend    │◀────│  React Frontend │
│   (Sensors)     │     │   (API/WS)      │     │   (Dashboard)   │
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │
                        ┌────────▼────────┐
                        │   PostgreSQL    │
                        │   (Database)    │
                        └─────────────────┘
```

## 📁 Структура проекту

```
diploma/
├── esp32_esp8266.ino      # ESP прошивка
├── backend/               # Go backend
│   ├── cmd/server/        # Main entry point
│   ├── internal/
│   │   ├── config/        # Configuration
│   │   ├── database/      # DB queries
│   │   ├── handlers/      # HTTP handlers
│   │   ├── middleware/    # Auth middleware
│   │   ├── models/        # Data models
│   │   ├── services/      # Business logic
│   │   └── websocket/     # WebSocket hub
│   ├── Dockerfile
│   └── go.mod
├── frontend/              # React frontend
│   ├── src/
│   │   ├── components/    # UI components
│   │   ├── pages/         # Page components
│   │   ├── services/      # API services
│   │   ├── hooks/         # Custom hooks
│   │   └── contexts/      # State management
│   ├── Dockerfile
│   └── package.json
├── docker-compose.yml     # Production
└── docker-compose.dev.yml # Development
```

## 🔌 API Endpoints

### Auth
- `GET /api/v1/auth/google` - Google OAuth login
- `GET /api/v1/auth/google/callback` - OAuth callback
- `POST /api/v1/auth/refresh` - Refresh JWT token

### Devices
- `GET /api/v1/devices` - List user devices
- `POST /api/v1/devices` - Create device
- `GET /api/v1/devices/:id` - Get device
- `DELETE /api/v1/devices/:id` - Delete device
- `POST /api/v1/devices/:id/regenerate-token` - New token
- `GET /api/v1/devices/:id/metrics` - Get metrics
- `POST /api/v1/devices/:id/commands` - Send command

### ESP Endpoints
- `POST /api/v1/metrics` - Push metrics (X-Device-Token)
- `GET /api/v1/devices/commands` - Get pending command
- `POST /api/v1/devices/commands/:id/ack` - Acknowledge command

### Dashboard
- `GET /api/v1/dashboard/stats` - Get statistics

### Admin
- `GET /api/v1/admin/users` - List all users
- `GET /api/v1/admin/devices` - List all devices

## 🛠️ Команди для ESP

| Command | Опис |
|---------|------|
| `reboot` | Перезавантаження пристрою |
| `toggle_dht` | Увімкнути/вимкнути DHT датчик |
| `toggle_mesh` | Увімкнути/вимкнути mesh мережу |
| `set_interval` | Змінити інтервал відправки метрик |
| `set_name` | Змінити назву пристрою |
| `update_firmware` | OTA оновлення прошивки |

## 🔐 Безпека

- JWT токени з терміном дії 7 днів
- Device токени - 64 символи hex
- HTTPS рекомендовано для production
- Google OAuth2 для авторизації
- CORS налаштування

## 🐛 Troubleshooting

### ESP не підключається
1. Перевір WiFi credentials
2. Перевір Backend URL (без trailing slash)
3. Перевір токен пристрою
4. Дивись Serial Monitor для помилок

### Backend не запускається
```bash
# Перевір логи
docker-compose logs backend

# Перевір з'єднання з БД
docker-compose exec postgres psql -U postgres -d iot_dashboard
```

### Frontend не показує дані
1. Перевір Network tab в DevTools
2. Перевір console.log
3. Перевір чи JWT токен валідний

## 📄 Ліцензія

MIT License - використовуй як хочеш!

## 🤝 Contributing

Pull requests welcome! Для великих змін спочатку відкрий issue.

---

Made with ❤️ for IoT enthusiasts

