#!/bin/bash

# Monitor ccusage resource usage
# Usage: ./monitor_ccusage.sh [ccusage|ccusage_go]
# Default: monitor ccusage (TypeScript version)
# This script tracks all child processes including Node.js runtime

PROGRAM=${1:-ccusage}

if [ "$PROGRAM" = "ccusage" ]; then
    echo "Monitoring: ccusage (TypeScript version)"
    # Start ccusage blocks --live in background and get PID
    script -q /dev/null npx -y ccusage@latest blocks --live &
    MAIN_PID=$!
elif [ "$PROGRAM" = "ccusage_go" ]; then
    echo "Monitoring: ccusage_go"
    # Start ccusage_go blocks --live in background with TTY
    script -q /dev/null ./ccusage_go blocks --live &
    MAIN_PID=$!
elif [ "$PROGRAM" = "ccusage_go_v2" ]; then
    echo "Monitoring: ccusage_go_v2 (optimized)"
    # Start ccusage_go_v2 blocks --live in background with TTY
    script -q /dev/null ./ccusage_go_v2 blocks --live &
    MAIN_PID=$!
elif [ "$PROGRAM" = "ccusage_go_v3" ]; then
    echo "Monitoring: ccusage_go_v3 (streaming)"
    # Start ccusage_go_v3 blocks --live in background with TTY
    script -q /dev/null ./ccusage_go_v3 blocks --live &
    MAIN_PID=$!
elif [ "$PROGRAM" = "ccusage_go_optimized" ]; then
    echo "Monitoring: ccusage_go_optimized (single worker, memory optimized)"
    # Start ccusage_go_optimized blocks --live in background with TTY
    script -q /dev/null ./ccusage_go_optimized blocks --live &
    MAIN_PID=$!
elif [ "$PROGRAM" = "ccusage_go_fixed" ]; then
    echo "Monitoring: ccusage_go_fixed (with cache token fix)"
    # Start ccusage_go_fixed blocks --live in background with TTY
    script -q /dev/null ./ccusage_go_fixed blocks --live &
    MAIN_PID=$!
else
    echo "Usage: $0 [ccusage|ccusage_go|ccusage_go_optimized|ccusage_go_fixed]"
    exit 1
fi

# Initialize variables
MAX_CPU=0
MAX_MEM=0
WARMUP=5
DURATION=15
TOTAL_TIME=$((WARMUP + DURATION))

echo "Warmup: ${WARMUP} seconds (skipping initial startup)"
echo "Monitoring duration: ${DURATION} seconds"
echo "Total time: ${TOTAL_TIME} seconds"
echo "----------------------------------------"

# Function to get all child processes recursively
get_all_pids() {
    local parent=$1
    local all_pids="$parent"
    local children=$(pgrep -P $parent 2>/dev/null)
    
    for child in $children; do
        all_pids="$all_pids $child"
        # Recursively get children of children
        local grandchildren=$(get_all_pids $child)
        all_pids="$all_pids $grandchildren"
    done
    
    echo $all_pids | tr ' ' '\n' | sort -u | tr '\n' ' '
}

# Warmup period - wait for initial startup
echo "Starting warmup period..."
sleep $WARMUP
echo "Warmup complete, starting monitoring..."
echo ""

# Monitor for specified duration
START_TIME=$(date +%s)
while [ $(($(date +%s) - START_TIME)) -lt $DURATION ]; do
    if ps -p $MAIN_PID > /dev/null 2>&1; then
        # Get all related PIDs (main process and all children)
        ALL_PIDS=$(get_all_pids $MAIN_PID)
        
        # Calculate total CPU and memory for all processes
        TOTAL_CPU=0
        TOTAL_MEM_KB=0
        PROCESS_INFO=""
        
        for pid in $ALL_PIDS; do
            if ps -p $pid > /dev/null 2>&1; then
                # Get process info
                PROC_INFO=$(ps -o pid,comm,pcpu,rss -p $pid | tail -n 1)
                PROC_NAME=$(echo $PROC_INFO | awk '{print $2}')
                PROC_CPU=$(echo $PROC_INFO | awk '{print $3}')
                PROC_MEM=$(echo $PROC_INFO | awk '{print $4}')
                
                # Handle special cases where ps might show non-numeric values
                if [[ ! "$PROC_CPU" =~ ^[0-9.]+$ ]]; then
                    PROC_CPU="0.0"
                fi
                if [[ ! "$PROC_MEM" =~ ^[0-9]+$ ]]; then
                    PROC_MEM="0"
                fi
                
                # Accumulate totals
                TOTAL_CPU=$(echo "$TOTAL_CPU + $PROC_CPU" | bc)
                TOTAL_MEM_KB=$(echo "$TOTAL_MEM_KB + $PROC_MEM" | bc)
                
                # Store process info for display
                if [ ! -z "$PROCESS_INFO" ]; then
                    PROCESS_INFO="${PROCESS_INFO}, "
                fi
                PROCESS_INFO="${PROCESS_INFO}${PROC_NAME}(${PROC_CPU}%)"
            fi
        done
        
        # Convert memory to MB
        TOTAL_MEM_MB=$(echo "scale=2; $TOTAL_MEM_KB / 1024" | bc)
        
        # Update maximum values
        if (( $(echo "$TOTAL_CPU > $MAX_CPU" | bc -l) )); then
            MAX_CPU=$TOTAL_CPU
            MAX_CPU_PROCS=$PROCESS_INFO
        fi
        
        if (( $(echo "$TOTAL_MEM_MB > $MAX_MEM" | bc -l) )); then
            MAX_MEM=$TOTAL_MEM_MB
            MAX_MEM_PROCS=$PROCESS_INFO
        fi
        
        # Real-time display
        printf "\rCurrent - CPU: %.1f%% | Memory: %.2f MB | Processes: %d        \n" \
               $TOTAL_CPU $TOTAL_MEM_MB $(echo $ALL_PIDS | wc -w)
        printf "Peak    - CPU: %.1f%% | Memory: %.2f MB                    \r" \
               $MAX_CPU $MAX_MEM
        
        sleep 0.5
    else
        echo -e "\nMain process ended"
        break
    fi
done

# Terminate all processes
for pid in $(get_all_pids $MAIN_PID); do
    kill $pid 2>/dev/null
done
wait $MAIN_PID 2>/dev/null

echo ""
echo ""
echo "----------------------------------------"
echo "MONITORING RESULTS (${DURATION}-second peak values after ${WARMUP}s warmup):"
echo "Peak CPU Usage: ${MAX_CPU}%"
echo "Peak Memory Usage: ${MAX_MEM} MB"
echo ""
echo "Processes at peak CPU: ${MAX_CPU_PROCS}"
echo "Processes at peak Memory: ${MAX_MEM_PROCS}"