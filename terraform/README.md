# 🚀 Terraform - Google Cloud Deployment

Супер-дешева інфраструктура для IoT Dashboard в Google Cloud.

## 💰 Вартість

| Компонент | Ціна/місяць |
|-----------|-------------|
| Cloud Run (Backend) | ~$0-5 (free tier: 2M requests) |
| Cloud Run (Frontend) | ~$0-2 |
| Cloud SQL db-f1-micro | ~$7-10 |
| Secret Manager | ~$0 (free tier) |
| Artifact Registry | ~$0.10/GB |
| **TOTAL** | **~$8-15/month** |

## 📋 Prerequisites

1. **Google Cloud Account** з білінгом
2. **gcloud CLI** встановлений
3. **Terraform** встановлений
4. **Docker** встановлений

### Встановлення інструментів (macOS)

```bash
# Terraform
brew install terraform

# Google Cloud SDK
brew install google-cloud-sdk

# Логін в GCP
gcloud auth login
gcloud auth application-default login
```

## 🚀 Deployment

### 1. Створи GCP проект

```bash
# Створи новий проект (або використай існуючий)
gcloud projects create iot-dashboard-xxx --name="IoT Dashboard"

# Увімкни білінг в консолі:
# https://console.cloud.google.com/billing
```

### 2. Налаштуй змінні

```bash
cd terraform

# Скопіюй приклад
cp terraform.tfvars.example terraform.tfvars

# Відредагуй
nano terraform.tfvars
```

Заповни:
- `project_id` - ID твого GCP проекту
- `google_client_id` - OAuth Client ID
- `google_client_secret` - OAuth Client Secret
- `admin_email` - твій email
- `jwt_secret` - випадковий рядок 32+ символів
- `db_password` - пароль для бази даних

### 3. Deploy!

```bash
./deploy.sh
```

Скрипт:
1. Створить інфраструктуру (Cloud SQL, Secrets, etc.)
2. Збілдить Docker images
3. Запушить в Artifact Registry
4. Задеплоїть на Cloud Run

### 4. Оновлення OAuth

Після деплою додай новий redirect URI в Google Cloud Console:
```
https://iot-backend-xxx-ew1.a.run.app/api/v1/auth/google/callback
```

## 🔧 Commands

```bash
# Ініціалізація
terraform init

# План змін
terraform plan

# Застосувати
terraform apply

# Видалити все
./destroy.sh
```

## 📱 ESP Configuration

Після деплою отримаєш URL для ESP:
```
Backend URL: https://iot-backend-xxx-ew1.a.run.app
```

## 🆓 Ще дешевший варіант (БЕЗКОШТОВНО!)

Якщо хочеш **повністю безкоштовно**, використай зовнішній PostgreSQL:

### Варіант 1: Supabase (рекомендую)
1. Зареєструйся на https://supabase.com
2. Створи проект (безкоштовно)
3. Скопіюй Connection String
4. Закоментуй Cloud SQL в `main.tf`
5. Використай Supabase URL в `DATABASE_URL`

### Варіант 2: Neon
1. Зареєструйся на https://neon.tech
2. Створи проект (безкоштовно)
3. Скопіюй Connection String

### Варіант 3: Railway
1. Зареєструйся на https://railway.app
2. Додай PostgreSQL (безкоштовно до ліміту)

## 🧹 Cleanup

```bash
# Видалити всю інфраструктуру
./destroy.sh
```

## 📊 Monitoring

- **Cloud Run**: https://console.cloud.google.com/run
- **Cloud SQL**: https://console.cloud.google.com/sql
- **Logs**: https://console.cloud.google.com/logs

