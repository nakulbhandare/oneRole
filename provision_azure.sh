#!/bin/bash
# OneRole Azure Provisioning Script
# Requirements: az CLI installed and logged in

RG_NAME="OneRole_RG"
VM_NAME="OneRole_Worker_VM"
LOCATION="eastus" # You can change this to a region closer to you

echo "Provisioning Azure resources..."

# 1. Create Resource Group
az group create --name $RG_NAME --location $LOCATION

# 2. Create VM (Ubuntu 22.04 LTS, Standard_B2s for good performance)
az vm create \
  --resource-group $RG_NAME \
  --name $VM_NAME \
  --image Ubuntu2204 \
  --size Standard_B2s \
  --admin-username azureuser \
  --generate-ssh-keys \
  --public-ip-sku Standard

# 3. Open necessary ports
az vm open-port \
  --resource-group $RG_NAME \
  --name $VM_NAME \
  --port 8082,8088,3001 --priority 1001

# 4. Get Public IP
IP=$(az vm show -d -g $RG_NAME -n $VM_NAME --query publicIps -o tsv)
echo "Provisioning complete!"
echo "VM Public IP: $IP"
echo "You can now SSH using: ssh azureuser@$IP"

