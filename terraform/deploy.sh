#!/bin/bash
set -e

# ==========================================
# IoT Dashboard - Deploy to Google Cloud
# With Load Balancer + Custom Domain
# ==========================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}🚀 IoT Dashboard - Deploy to GCP${NC}"
echo "=================================="

# Check terraform.tfvars
if [ ! -f "terraform.tfvars" ]; then
    echo -e "${RED}❌ terraform.tfvars not found!${NC}"
    echo "Run: cp terraform.tfvars.example terraform.tfvars"
    exit 1
fi

# Parse variables
PROJECT_ID=$(grep 'project_id' terraform.tfvars | cut -d'"' -f2)
REGION=$(grep 'region' terraform.tfvars | cut -d'"' -f2)
DOMAIN=$(grep 'domain' terraform.tfvars | cut -d'"' -f2)

echo -e "${BLUE}📋 Project: $PROJECT_ID${NC}"
echo -e "${BLUE}📍 Region:  $REGION${NC}"
echo -e "${BLUE}🌐 Domain:  $DOMAIN${NC}"
echo ""

# GCP Auth
echo -e "${GREEN}🔐 Checking GCP authentication...${NC}"
gcloud auth application-default print-access-token > /dev/null 2>&1 || {
    echo "Authenticating with GCP..."
    gcloud auth application-default login
}

gcloud config set project $PROJECT_ID

# Terraform
echo -e "${GREEN}🔧 Initializing Terraform...${NC}"
terraform init

echo -e "${GREEN}📝 Planning...${NC}"
terraform plan -out=tfplan

echo ""
read -p "Apply? (yes/no): " confirm
if [ "$confirm" != "yes" ]; then
    echo "Cancelled."
    exit 0
fi

echo -e "${GREEN}🏗️  Creating infrastructure...${NC}"
terraform apply tfplan

# Get outputs
REGISTRY=$(terraform output -raw artifact_registry)
LB_IP=$(terraform output -raw load_balancer_ip)

echo ""
echo -e "${GREEN}✅ Infrastructure created!${NC}"
echo ""

# Build and push images
echo -e "${GREEN}🐳 Building Docker images...${NC}"

gcloud auth configure-docker ${REGION}-docker.pkg.dev --quiet

# Backend
echo -e "${YELLOW}Building backend...${NC}"
cd ../backend
docker build --platform linux/amd64 -t ${REGISTRY}/backend:latest .
docker push ${REGISTRY}/backend:latest

# Frontend - з API через той самий домен
echo -e "${YELLOW}Building frontend...${NC}"
cd ../frontend

# Nginx config для проксі через LB
cat > nginx.conf << 'EOF'
server {
    listen 80;
    server_name localhost;
    root /usr/share/nginx/html;
    index index.html;

    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
}
EOF

docker build --platform linux/amd64 -t ${REGISTRY}/frontend:latest .
docker push ${REGISTRY}/frontend:latest

cd ../terraform

# Deploy to Cloud Run
echo -e "${GREEN}🔄 Deploying to Cloud Run...${NC}"

gcloud run deploy iot-backend \
    --image ${REGISTRY}/backend:latest \
    --region $REGION \
    --quiet

gcloud run deploy iot-frontend \
    --image ${REGISTRY}/frontend:latest \
    --region $REGION \
    --quiet

# Output
echo ""
echo -e "${GREEN}🎉 DEPLOYMENT COMPLETE!${NC}"
echo "=================================="
echo ""
echo -e "${YELLOW}📍 STEP 1: Configure DNS${NC}"
echo -e "   Add A record to your domain:"
echo ""
echo -e "   Type:  ${GREEN}A${NC}"
echo -e "   Name:  ${GREEN}$DOMAIN${NC}"
echo -e "   Value: ${GREEN}$LB_IP${NC}"
echo ""
echo -e "${YELLOW}📍 STEP 2: Wait for SSL (15-60 min)${NC}"
echo -e "   Check status:"
echo -e "   gcloud compute ssl-certificates describe iot-ssl-cert --global"
echo ""
echo -e "${YELLOW}📍 STEP 3: Update Google OAuth${NC}"
echo -e "   Add redirect URI:"
echo -e "   ${GREEN}https://$DOMAIN/api/v1/auth/google/callback${NC}"
echo ""
echo -e "${YELLOW}📱 ESP Configuration:${NC}"
echo -e "   Backend URL: ${GREEN}https://$DOMAIN${NC}"
echo ""
terraform output estimated_monthly_cost
