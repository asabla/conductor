#!/usr/bin/env bash
# =============================================================================
# wait-for-it.sh - Wait for a service to be available
# =============================================================================
# Use this script to wait for another service to become available
#
# Usage:
#   ./wait-for-it.sh host:port [-t timeout] [-- command args]
#   ./wait-for-it.sh -h host -p port [-t timeout] [-- command args]
#
# Examples:
#   ./wait-for-it.sh postgres:5432 -- echo "Postgres is up"
#   ./wait-for-it.sh -h localhost -p 5432 -t 60 -- npm start
#
# Environment variables:
#   WAIT_HOSTS     - Comma-separated list of host:port to wait for
#   WAIT_TIMEOUT   - Timeout in seconds (default: 30)
# =============================================================================

set -e

# Default values
WAITFORIT_TIMEOUT=${WAIT_TIMEOUT:-30}
WAITFORIT_STRICT=0
WAITFORIT_QUIET=0
WAITFORIT_CHILD=0
WAITFORIT_HOST=""
WAITFORIT_PORT=""
WAITFORIT_CMD=()

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echoerr() {
    if [[ $WAITFORIT_QUIET -ne 1 ]]; then
        echo -e "$@" 1>&2
    fi
}

usage() {
    cat << USAGE >&2
Usage:
    $0 host:port [-t timeout] [-- command args]
    $0 -h HOST -p PORT [-t timeout] [-s] [-q] [-- command args]

Options:
    -h HOST     Host or IP to wait for
    -p PORT     TCP port to wait for
    -t TIMEOUT  Timeout in seconds (default: 30)
    -s          Strict mode: exit with error if timeout occurs
    -q          Quiet mode: don't output any messages
    --          Execute command after the wait is complete

Environment:
    WAIT_HOSTS    Comma-separated list of host:port to wait for
    WAIT_TIMEOUT  Timeout in seconds (default: 30)

Examples:
    $0 postgres:5432 -t 60 -- echo "Postgres is ready"
    $0 -h redis -p 6379 -s -- redis-cli ping
    WAIT_HOSTS=pg:5432,redis:6379 $0 -- ./start-app.sh
USAGE
    exit 1
}

wait_for() {
    local host=$1
    local port=$2
    local timeout=$3
    
    if [[ $timeout -gt 0 ]]; then
        echoerr "${YELLOW}Waiting up to ${timeout}s for ${host}:${port}...${NC}"
    else
        echoerr "${YELLOW}Waiting for ${host}:${port} without timeout...${NC}"
    fi
    
    local start_ts=$(date +%s)
    
    while :; do
        if nc -z "$host" "$port" 2>/dev/null; then
            local end_ts=$(date +%s)
            echoerr "${GREEN}${host}:${port} is available after $((end_ts - start_ts))s${NC}"
            return 0
        fi
        
        if [[ $timeout -gt 0 ]]; then
            local now_ts=$(date +%s)
            if [[ $((now_ts - start_ts)) -ge $timeout ]]; then
                echoerr "${RED}Timeout after ${timeout}s waiting for ${host}:${port}${NC}"
                return 1
            fi
        fi
        
        sleep 1
    done
}

wait_for_wrapper() {
    local result=0
    
    if [[ $WAITFORIT_CHILD -gt 0 ]]; then
        wait_for "$WAITFORIT_HOST" "$WAITFORIT_PORT" "$WAITFORIT_TIMEOUT" &
        local pid=$!
        trap "kill -INT $pid 2>/dev/null" INT
        wait $pid
        result=$?
        trap - INT
    else
        wait_for "$WAITFORIT_HOST" "$WAITFORIT_PORT" "$WAITFORIT_TIMEOUT"
        result=$?
    fi
    
    return $result
}

# Parse WAIT_HOSTS environment variable
parse_wait_hosts() {
    if [[ -n "$WAIT_HOSTS" ]]; then
        IFS=',' read -ra HOSTS <<< "$WAIT_HOSTS"
        for hostport in "${HOSTS[@]}"; do
            hostport=$(echo "$hostport" | xargs)  # trim whitespace
            if [[ "$hostport" == *":"* ]]; then
                local host="${hostport%%:*}"
                local port="${hostport#*:}"
                wait_for "$host" "$port" "$WAITFORIT_TIMEOUT"
                if [[ $? -ne 0 ]] && [[ $WAITFORIT_STRICT -eq 1 ]]; then
                    exit 1
                fi
            fi
        done
    fi
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        *:* )
            WAITFORIT_HOST="${1%%:*}"
            WAITFORIT_PORT="${1#*:}"
            shift
            ;;
        -h)
            WAITFORIT_HOST="$2"
            shift 2
            ;;
        -p)
            WAITFORIT_PORT="$2"
            shift 2
            ;;
        -t)
            WAITFORIT_TIMEOUT="$2"
            shift 2
            ;;
        -s)
            WAITFORIT_STRICT=1
            shift
            ;;
        -q)
            WAITFORIT_QUIET=1
            shift
            ;;
        --)
            shift
            WAITFORIT_CMD=("$@")
            break
            ;;
        --help)
            usage
            ;;
        *)
            echoerr "${RED}Unknown argument: $1${NC}"
            usage
            ;;
    esac
done

# Handle WAIT_HOSTS environment variable
if [[ -n "$WAIT_HOSTS" ]]; then
    parse_wait_hosts
fi

# Wait for specific host:port if provided
if [[ -n "$WAITFORIT_HOST" ]] && [[ -n "$WAITFORIT_PORT" ]]; then
    wait_for_wrapper
    WAITFORIT_RESULT=$?
    
    if [[ $WAITFORIT_RESULT -ne 0 ]] && [[ $WAITFORIT_STRICT -eq 1 ]]; then
        echoerr "${RED}Strict mode: exiting due to timeout${NC}"
        exit $WAITFORIT_RESULT
    fi
fi

# Execute command if provided
if [[ ${#WAITFORIT_CMD[@]} -gt 0 ]]; then
    echoerr "${GREEN}Executing command: ${WAITFORIT_CMD[*]}${NC}"
    exec "${WAITFORIT_CMD[@]}"
fi
