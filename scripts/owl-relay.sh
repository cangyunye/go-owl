#!/bin/bash

csv_quote() {
    local field="$1"
    if [[ "$field" =~ [,\"$'\n'] ]]; then
        field="${field//\"/\"\"}"
        printf '"%s"' "$field"
    else
        printf '%s' "$field"
    fi
}

now_ns() {
    date +%s%N 2>/dev/null || echo 0
}

SOURCE=""
TARGETS=""
TIMEOUT=30
PASSWORDS=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --source)
            SOURCE="$2"
            shift 2
            ;;
        --targets)
            TARGETS="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --passwords)
            PASSWORDS="$2"
            shift 2
            ;;
        *)
            echo "owl-relay: unknown option: $1" >&2
            exit 2
            ;;
    esac
done

if [[ -z "$SOURCE" ]]; then
    echo "owl-relay: --source is required" >&2
    exit 2
fi

if [[ ! -f "$SOURCE" ]]; then
    echo "owl-relay: source file not found: $SOURCE" >&2
    exit 2
fi

if [[ -z "$TARGETS" ]]; then
    echo "owl-relay: --targets is required" >&2
    exit 2
fi

if [[ ! "$TIMEOUT" =~ ^[0-9]+$ ]]; then
    echo "owl-relay: --timeout must be a positive integer" >&2
    exit 2
fi

IFS=',' read -ra TARGET_ARR <<< "$TARGETS"
IFS=',' read -ra PASSWORD_ARR <<< "$PASSWORDS"

target_count=${#TARGET_ARR[@]}
password_count=${#PASSWORD_ARR[@]}

if [[ -n "$PASSWORDS" ]] && [[ $password_count -ne $target_count ]]; then
    echo "owl-relay: --passwords count ($password_count) does not match --targets count ($target_count)" >&2
    exit 2
fi

echo "target,status,error,duration_ms"

success_count=0
fail_count=0

for i in "${!TARGET_ARR[@]}"; do
    target="${TARGET_ARR[$i]}"
    password="${PASSWORD_ARR[$i]:-}"

    scp_opts=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10 -o BatchMode=no)

    if [[ -n "$password" ]]; then
        if ! command -v sshpass &>/dev/null; then
            target_csv="$(csv_quote "$target")"
            echo "$target_csv,auth_failed,$(csv_quote 'sshpass not available'),0"
            ((fail_count++))
            continue
        fi

        start_ns=$(now_ns)
        SSHPASS="$password" timeout "$TIMEOUT" sshpass -e scp "${scp_opts[@]}" "$SOURCE" "$target" &>/dev/null
        scp_exit=$?
        end_ns=$(now_ns)
    else
        start_ns=$(now_ns)
        timeout "$TIMEOUT" scp "${scp_opts[@]}" "$SOURCE" "$target" &>/dev/null
        scp_exit=$?
        end_ns=$(now_ns)
    fi

    if [[ $start_ns -gt 0 ]] && [[ $end_ns -gt 0 ]]; then
        duration_ms=$(( (end_ns - start_ns) / 1000000 ))
    else
        duration_ms=0
    fi

    case $scp_exit in
        0)
            status="success"
            error=""
            ((success_count++))
            ;;
        124)
            status="timeout"
            error="timeout after ${TIMEOUT}s"
            ((fail_count++))
            ;;
        *)
            status="failed"
            error="scp exit code $scp_exit"
            ((fail_count++))
            ;;
    esac

    target_csv="$(csv_quote "$target")"
    error_csv="$(csv_quote "$error")"
    echo "$target_csv,$status,$error_csv,$duration_ms"
done

if [[ $fail_count -eq 0 ]]; then
    exit 0
elif [[ $success_count -gt 0 ]]; then
    exit 1
else
    exit 2
fi
