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

url_encode() {
    local str="$1"
    local encoded=""
    local i c
    for (( i = 0; i < ${#str}; i++ )); do
        c="${str:$i:1}"
        case "$c" in
            [a-zA-Z0-9.~_-]) encoded+="$c" ;;
            *) encoded+=$(printf '%%%02X' "'$c") ;;
        esac
    done
    printf '%s' "$encoded"
}

parse_target_to_url() {
    local target="$1"
    local password="$2"

    local user_host="${target%%:*}"
    local remote_path="${target#*:}"

    local user="${user_host%%@*}"
    local host="${user_host#*@}"

    local port=22
    if [[ "$host" =~ ^(.+):([0-9]+)$ ]]; then
        host="${BASH_REMATCH[1]}"
        port="${BASH_REMATCH[2]}"
    fi

    if [[ -n "$password" ]]; then
        local encoded_pass
        encoded_pass="$(url_encode "$password")"
        printf 'scp://%s:%s@%s:%s%s' "$user" "$encoded_pass" "$host" "$port" "$remote_path"
    else
        printf 'scp://%s@%s:%s%s' "$user" "$host" "$port" "$remote_path"
    fi
}

SOURCE=""
TARGETS=""
TIMEOUT=30
PASSWORDS=""
GSCP_PATH="/tmp/gscp"

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
        --gscp-path)
            GSCP_PATH="$2"
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

if [[ ! -x "$GSCP_PATH" ]]; then
    echo "owl-relay: gscp not found or not executable: $GSCP_PATH" >&2
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

    url="$(parse_target_to_url "$target" "$password")"

    start_ns=$(now_ns)
    timeout "$TIMEOUT" "$GSCP_PATH" -m PUT -q "$SOURCE" "$url" &>/dev/null
    scp_exit=$?
    end_ns=$(now_ns)

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
            error="gscp exit code $scp_exit"
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
