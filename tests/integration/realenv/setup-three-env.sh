#!/bin/bash
set -euo pipefail

# Three-Environment Setup for Real Environment Tests
# Uses existing fixtures from tests/integration/projects/magento2/
#
# Usage:
#   ./setup-three-env.sh              # Setup full environment
#   ./setup-three-env.sh cleanup      # Cleanup only
#   ./setup-three-env.sh verify       # Verify existing environment

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
SSH_KEY_DIR="${SCRIPT_DIR}/.ssh"
FIXTURES_DIR="${PROJECT_ROOT}/tests/integration/projects/magento2"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# Determine docker compose command
if docker compose version >/dev/null 2>&1; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

cleanup() {
    log "Cleaning up existing environment..."
    $DOCKER_COMPOSE -f "${SCRIPT_DIR}/docker-compose.three-env.yml" down -v 2>/dev/null || true
    docker network rm govard-test-net 2>/dev/null || true
    log "Cleanup complete"
}

setup_ssh_keys() {
    log "Setting up SSH keys..."
    mkdir -p "${SSH_KEY_DIR}"
    
    if [[ ! -f "${SSH_KEY_DIR}/id_rsa" ]]; then
        log "Generating new SSH key pair..."
        ssh-keygen -t rsa -b 4096 -f "${SSH_KEY_DIR}/id_rsa" -N "" -C "govard-test"
    else
        log "Using existing SSH key pair..."
    fi
    
    # Ensure proper permissions
    chmod 600 "${SSH_KEY_DIR}/id_rsa"
    chmod 644 "${SSH_KEY_DIR}/id_rsa.pub"
    
    log "SSH keys ready at: ${SSH_KEY_DIR}"
}

wait_for_mysql() {
    local container=$1
    local max_attempts=30
    local attempt=1
    
    log "Waiting for MySQL in ${container}..."
    while [ $attempt -le $max_attempts ]; do
        if docker exec "${container}" mysql -uroot -proot -e "SELECT 1" >/dev/null 2>&1; then
            log "MySQL in ${container} is ready!"
            return 0
        fi
        log "Attempt ${attempt}/${max_attempts}: MySQL not ready yet, waiting..."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    error "MySQL in ${container} failed to start after ${max_attempts} attempts"
    return 1
}

wait_for_ssh() {
    local port=$1
    local max_attempts=30
    local attempt=1
    
    log "Waiting for SSH on port ${port}..."
    while [ $attempt -le $max_attempts ]; do
        if nc -z localhost ${port} 2>/dev/null; then
            log "SSH on port ${port} is ready!"
            return 0
        fi
        log "Attempt ${attempt}/${max_attempts}: SSH not ready yet, waiting..."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    error "SSH on port ${port} failed to start after ${max_attempts} attempts"
    return 1
}

start_environment() {
    log "Starting Docker environment..."
    
    # Verify fixtures exist
    for fixture in options-local options-dev options-staging; do
        if [[ ! -d "${FIXTURES_DIR}/${fixture}" ]]; then
            error "Fixture not found: ${FIXTURES_DIR}/${fixture}"
            exit 1
        fi
        if [[ ! -f "${FIXTURES_DIR}/${fixture}/init.sql" ]]; then
            error "Database init file not found: ${FIXTURES_DIR}/${fixture}/init.sql"
            exit 1
        fi
    done
    log "Using fixtures from: ${FIXTURES_DIR}"
    
    # Create shared SSH volume if it doesn't exist
    docker volume create govard-test_ssh 2>/dev/null || true
    
    # Start services
    $DOCKER_COMPOSE -f "${SCRIPT_DIR}/docker-compose.three-env.yml" up -d
    
    log "Waiting for services to be healthy..."
    
    # Wait for MySQL instances
    wait_for_mysql "m2-clone-basic-db-1" || exit 1
    wait_for_mysql "govard-test-dev-db" || exit 1
    wait_for_mysql "govard-test-staging-db" || exit 1
    
    # Configure MySQL users for compatibility with mariadb-client (which doesn't support caching_sha2_password well)
    log "Configuring MySQL users for compatibility..."
    docker exec m2-clone-basic-db-1 mysql -uroot -proot -e "ALTER USER 'magento'@'%' IDENTIFIED WITH mysql_native_password BY 'magento';"
    docker exec govard-test-dev-db mysql -uroot -proot -e "ALTER USER 'magento'@'%' IDENTIFIED WITH mysql_native_password BY 'magento';"
    docker exec govard-test-staging-db mysql -uroot -proot -e "ALTER USER 'magento'@'%' IDENTIFIED WITH mysql_native_password BY 'magento';"
    
    # Wait for SSH
    wait_for_ssh 9022 || exit 1
    wait_for_ssh 9023 || exit 1
    wait_for_ssh 9024 || exit 1
    
    # Populate code from fixtures
    log "Populating environment code from fixtures..."
    for container in m2-clone-basic-php-1 govard-test-dev-php govard-test-staging-php; do
        docker exec $container sh -c "cp -r /fixtures/* /var/www/html/ 2>/dev/null || true"
        # Create mock bin/magento because some build steps require it to exist
        docker exec $container sh -c "mkdir -p /var/www/html/bin && echo '#!/bin/sh\necho \"Mock Magento CLI\"' > /var/www/html/bin/magento && chmod +x /var/www/html/bin/magento"
    done
    
    log "All services are ready!"
}

setup_ssh_access() {
    log "Setting up SSH access on containers..."
    
    # Copy SSH key to all containers
    for container in govard-test-local-ssh govard-test-dev-ssh govard-test-staging-ssh; do
        # Create .ssh directory for linuxserver.io user
        docker exec $container mkdir -p /config/.ssh
        
        # Copy public key
        cat "${SSH_KEY_DIR}/id_rsa.pub" | docker exec -i $container sh -c "cat > /config/.ssh/authorized_keys"
        
        # Set permissions
        docker exec $container chmod 700 /config/.ssh
        docker exec $container chmod 600 /config/.ssh/authorized_keys
        docker exec $container chown -R linuxserver.io:linuxserver.io /config/.ssh 2>/dev/null || true
        
        # Enable PermitRootLogin for tests that need root
        docker exec $container sh -c "echo 'PermitRootLogin yes' >> /config/sshd/sshd_config" 2>/dev/null || true
        
        # Copy authorized_keys to root as well
        docker exec $container mkdir -p /root/.ssh
        cat "${SSH_KEY_DIR}/id_rsa.pub" | docker exec -i $container sh -c "cat > /root/.ssh/authorized_keys"
        docker exec $container chmod 700 /root/.ssh
        docker exec $container chmod 600 /root/.ssh/authorized_keys
        
        # Install tools
        docker exec $container apk add --no-cache rsync mysql-client php83 php83-bcmath php83-ctype php83-curl php83-dom php83-fileinfo php83-gd php83-gettext php83-iconv php83-intl php83-mbstring php83-openssl php83-pdo php83-pdo_mysql php83-simplexml php83-soap php83-tokenizer php83-xml php83-xmlwriter php83-xsl php83-zip php83-sockets php83-session php83-zlib 2>/dev/null || true
        # Create symlink for php if it doesn't exist
        docker exec $container ln -sf /usr/bin/php83 /usr/bin/php 2>/dev/null || true
        
        log "SSH configured for ${container}"
    done
    
    # Restart SSH to apply config changes
    for container in govard-test-local-ssh govard-test-dev-ssh govard-test-staging-ssh; do
        docker exec $container pkill sshd 2>/dev/null || true
    done
    
    # Wait for SSH to restart
    sleep 3
}

verify_environment() {
    log "Verifying environment..."
    
    # Test SSH connectivity (linuxserver.io openssh-server uses 'linuxserver.io' user)
    log "Testing SSH connectivity..."
    
    # DEV SSH
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 \
        -i "${SSH_KEY_DIR}/id_rsa" \
        -p 9023 linuxserver.io@localhost "echo 'DEV_SSH_OK'" 2>/dev/null | grep -q "DEV_SSH_OK"; then
        log "✓ DEV SSH connection successful"
    else
        error "✗ DEV SSH connection failed"
        return 1
    fi
    
    # STAGING SSH
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 \
        -i "${SSH_KEY_DIR}/id_rsa" \
        -p 9024 linuxserver.io@localhost "echo 'STAGING_SSH_OK'" 2>/dev/null | grep -q "STAGING_SSH_OK"; then
        log "✓ STAGING SSH connection successful"
    else
        error "✗ STAGING SSH connection failed"
        return 1
    fi
    
    # Test database connectivity
    log "Testing database connectivity..."
    
    if docker exec m2-clone-basic-db-1 mysql -umagento -pmagento -e "SELECT 'LOCAL_DB_OK'" 2>/dev/null | grep -q "LOCAL_DB_OK"; then
        log "✓ LOCAL DB connection successful"
    else
        error "✗ LOCAL DB connection failed"
        return 1
    fi
    
    if docker exec govard-test-dev-db mysql -umagento -pmagento -e "SELECT 'DEV_DB_OK'" 2>/dev/null | grep -q "DEV_DB_OK"; then
        log "✓ DEV DB connection successful"
    else
        error "✗ DEV DB connection failed"
        return 1
    fi
    
    if docker exec govard-test-staging-db mysql -umagento -pmagento -e "SELECT 'STAGING_DB_OK'" 2>/dev/null | grep -q "STAGING_DB_OK"; then
        log "✓ STAGING DB connection successful"
    else
        error "✗ STAGING DB connection failed"
        return 1
    fi
    
    log "Environment verification complete!"
}

clear_known_hosts() {
    # Clear known_hosts to avoid man-in-the-middle warnings
    ssh-keygen -f "${HOME}/.ssh/known_hosts" -R '[localhost]:9022' 2>/dev/null || true
    ssh-keygen -f "${HOME}/.ssh/known_hosts" -R '[localhost]:9023' 2>/dev/null || true
    ssh-keygen -f "${HOME}/.ssh/known_hosts" -R '[localhost]:9024' 2>/dev/null || true
}

print_summary() {
    log ""
    log "========================================="
    log "Setup complete! Environment details:"
    log "========================================="
    log ""
    log "CONTAINERS:"
    log "  LOCAL:   m2-clone-basic-php-1, m2-clone-basic-db-1, govard-test-local-ssh"
    log "  DEV:     govard-test-dev-php, govard-test-dev-db, govard-test-dev-ssh"
    log "  STAGING: govard-test-staging-php, govard-test-staging-db, govard-test-staging-ssh"
    log ""
    log "FIXTURES:"
    log "  Using: ${FIXTURES_DIR}/options-local"
    log "  Using: ${FIXTURES_DIR}/options-dev"
    log "  Using: ${FIXTURES_DIR}/options-staging"
    log ""
    log "ACCESS:"
    log "  LOCAL:   SSH localhost:9022 (user: linuxserver.io), DB localhost:3306"
    log "  DEV:     SSH localhost:9023 (user: linuxserver.io), DB localhost:3307"
    log "  STAGING: SSH localhost:9024 (user: linuxserver.io), DB localhost:3308"
    log ""
    log "CONFIGURATION:"
    log "  SSH Key: ${SSH_KEY_DIR}/id_rsa"
    log ""
    log "EXAMPLE USAGE:"
    log "  # Test SSH to DEV:"
    log "  ssh -i ${SSH_KEY_DIR}/id_rsa -p 9023 linuxserver.io@localhost"
    log ""
    log "  # Run real environment tests:"
    log "  make test-realenv"
    log "========================================="
}

# Main execution
main() {
    log "Starting three-environment setup for Govard validation..."
    log "Using existing fixtures from: ${FIXTURES_DIR}"
    
    # Check prerequisites
    if ! command -v docker >/dev/null 2>&1; then
        error "Docker is not installed or not in PATH"
        exit 1
    fi
    
    if ! command -v docker-compose >/dev/null 2>&1 && ! docker compose version >/dev/null 2>&1; then
        error "Docker Compose is not installed"
        exit 1
    fi
    
    cleanup
    setup_ssh_keys
    clear_known_hosts
    start_environment
    setup_ssh_access
    verify_environment
    print_summary
}

# Handle arguments
case "${1:-}" in
    cleanup)
        cleanup
        ;;
    verify)
        verify_environment
        ;;
    *)
        main
        ;;
esac
