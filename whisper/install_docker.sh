#!/bin/bash

# Docker and Docker Compose Installation Script for Ubuntu 24.04 LTS
# Run with: bash install_docker.sh

set -e  # Exit on any error

echo "========================================="
echo "Docker Installation Script for Ubuntu 24.04"
echo "========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_status "Starting Docker installation process..."

# Update package index
print_status "Updating package index..."
apt update

# Install prerequisite packages
print_status "Installing prerequisite packages..."
apt install -y \
    ca-certificates \
    curl \
    gnupg \
    lsb-release

# Remove old Docker versions if they exist
print_status "Removing old Docker versions (if any)..."
apt remove -y docker docker-engine docker.io containerd runc 2>/dev/null || true

# Add Docker's official GPG key
print_status "Adding Docker's official GPG key..."
mkdir -p /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

# Add Docker repository
print_status "Adding Docker repository..."
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

# Update package index with Docker repository
print_status "Updating package index with Docker repository..."
apt update

# Install Docker Engine, CLI, and containerd
print_status "Installing Docker Engine, CLI, and containerd..."
apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# Start and enable Docker service
print_status "Starting and enabling Docker service..."
systemctl start docker
systemctl enable docker

# Add actual user to docker group
print_status "Adding user ($USER) to docker group..."
usermod -aG docker $USER

# Verify Docker installation
print_status "Verifying Docker installation..."
if docker --version; then
    print_status "Docker installed successfully!"
    docker --version
else
    print_error "Docker installation failed!"
    exit 1
fi

# Verify Docker Compose installation
print_status "Verifying Docker Compose installation..."
if docker compose version; then
    print_status "Docker Compose installed successfully!"
    docker compose version
else
    print_error "Docker Compose installation failed!"
    exit 1
fi

# Test Docker with hello-world
print_status "Testing Docker with hello-world container..."
if docker run --rm hello-world > /dev/null 2>&1; then
    print_status "Docker test successful!"
else
    print_warning "Docker test failed, but installation appears complete."
fi

echo ""
echo "========================================="
print_status "Installation completed successfully!"
echo "========================================="
echo ""
print_warning "IMPORTANT: You need to log out and log back in (or restart your system)"
print_warning "for the docker group changes to take effect for user: $USER"
echo ""
print_status "After logging back in, user $USER can run Docker commands without sudo:"
echo "  docker --version"
echo "  docker compose version"
echo "  docker run hello-world"
echo ""
print_status "Docker and Docker Compose are now installed and ready to use!"

# Optional: Show Docker info
echo ""
read -p "Would you like to see Docker system information? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    docker system info
fi