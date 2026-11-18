#!/bin/bash
VM_IP="40.116.109.236"
echo "Setting up Cloudflare Tunnel on $VM_IP..."

ssh azureuser@$VM_IP << 'REMOTE'
  # Install cloudflared
  wget https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb
  sudo dpkg -i cloudflared-linux-amd64.deb
  
  # Run the tunnel in background
  # We use nohup to keep it running after we disconnect
  nohup cloudflared tunnel --url http://localhost:80 > cloudflare.log 2>&1 &
  
  echo "Cloudflare tunnel started in background."
  echo "Waiting for tunnel URL (check cloudflare.log)..."
  sleep 5
  grep -o 'https://[-a-zA-Z0-9]*\.trycloudflare\.com' cloudflare.log
REMOTE
