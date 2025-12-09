#!/bin/bash
set -e

# ==========================================
# IoT Dashboard - Destroy Infrastructure
# ==========================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${RED}⚠️  WARNING: This will DESTROY all infrastructure!${NC}"
echo "=================================="
echo ""

read -p "Are you sure? Type 'destroy' to confirm: " confirm
if [ "$confirm" != "destroy" ]; then
    echo "Cancelled."
    exit 0
fi

echo -e "${YELLOW}🗑️  Destroying infrastructure...${NC}"
terraform destroy -auto-approve

echo -e "${GREEN}✅ Infrastructure destroyed.${NC}"

