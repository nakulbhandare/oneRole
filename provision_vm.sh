#!/bin/bash
VM_IP="40.116.109.236"
echo "Setting up VM at $VM_IP..."
ssh azureuser@$VM_IP << 'REMOTE'
  # Update and install dependencies
  sudo apt-get update
  sudo apt-get install -y docker.io docker-compose git
  sudo usermod -aG docker azureuser
  
  # Remove existing repo and containers if they exist
  sudo docker stop onerole-app 2>/dev/null || true
  sudo docker rm onerole-app 2>/dev/null || true
  rm -rf oneRole
  
  # Clone and build
  git clone https://github.com/nakulbhandare/oneRole.git
  cd oneRole
  
  # Build and run with lowercase tag
  sudo docker build -t onerole-app .
  sudo docker run -d -p 80:8082 --name onerole-app onerole-app
REMOTE
echo "Provisioning complete! Your app should be live at http://$VM_IP"