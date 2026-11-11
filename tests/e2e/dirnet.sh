#!/bin/sh
# Copyright AGNTCY Contributors (https://github.com/agntcy)
# SPDX-License-Identifier: Apache-2.0
#
# Starts or stops two peered dir daemons for network-mode testing.
#
#   dirnet.sh start DATA_DIR   start "main" (localhost:8878) and "peer"
#                              (localhost:8898, bootstrapped to main)
#   dirnet.sh seed             push and publish sample OASF 1.1.0 records:
#                              four on peer, one on main
#   dirnet.sh stop DATA_DIR    stop both
#
# DIRCTL_BIN must point at dirctl. Each node lives in DATA_DIR/{main,peer} with
# its log next to it, so the default daemon and ~/.agntcy/dir are never
# touched. Without --config the daemon uses built-in defaults, so each node is
# configured through env overrides on non-default ports. Short intervals make
# publication, sync and indexing finish quickly.
set -eu

: "${DIRCTL_BIN:?DIRCTL_BIN must be set}"
usage="usage: dirnet.sh start|stop DATA_DIR | dirnet.sh seed"
cmd="${1:?$usage}"
dir="${2:-}"
[ "$cmd" = seed ] || [ -n "$dir" ] || { echo "$usage" >&2; exit 2; }

# daemon status exits 0 either way; only its message tells.
running() {
  "$DIRCTL_BIN" daemon status --data-dir "$1" 2>&1 | grep -q 'is running'
}

wait_running() {
  for _ in $(seq 1 30); do
    running "$1" && return 0
    sleep 1
  done
  echo "ERROR: daemon in $1 failed to start; last log lines:" >&2
  tail -n 20 "$1.log" >&2 || true
  return 1
}

# start_node NAME API ZOT_PORT P2P_PORT GATEWAY_PORT BOOTSTRAP
start_node() {
  data="$dir/$1"
  if running "$data"; then
    echo "$1 already running on $2"
    return 0
  fi
  mkdir -p "$data"
  env DIRECTORY_DAEMON_SERVER_LISTEN_ADDRESS="$2" \
    DIRECTORY_DAEMON_SERVER_STORE_OCI_REGISTRY_ADDRESS="localhost:$3" \
    DIRECTORY_DAEMON_RECONCILER_LOCAL_REGISTRY_REGISTRY_ADDRESS="localhost:$3" \
    DIRECTORY_DAEMON_SERVER_ROUTING_LISTEN_ADDRESS="/ip4/127.0.0.1/tcp/$4" \
    DIRECTORY_DAEMON_SERVER_ROUTING_BOOTSTRAP_PEERS="$6" \
    DIRECTORY_DAEMON_SERVER_ROUTING_DIRECTORY_API_ADDRESS="$2" \
    DIRECTORY_DAEMON_SERVER_HTTP_GATEWAY_LISTEN_ADDRESS=":$5" \
    DIRECTORY_DAEMON_SERVER_HTTP_GATEWAY_PUBLIC_URL="http://localhost:$5" \
    DIRECTORY_DAEMON_SERVER_PUBLICATION_SCHEDULER_INTERVAL=2s \
    DIRECTORY_DAEMON_RECONCILER_REGSYNC_INTERVAL=2s \
    DIRECTORY_DAEMON_RECONCILER_INDEXER_INTERVAL=2s \
    nohup "$DIRCTL_BIN" daemon start --data-dir "$data" >>"$data.log" 2>&1 &
  wait_running "$data"
  echo "$1 running on $2"
}

# seed_record ADDR NAME DESCRIPTION SKILLS_JSON DOMAINS_JSON
seed_record() {
  record=$(printf '{"name":"%s","version":"0.1.0","schema_version":"1.1.0","description":"%s","authors":["lazydir-dev"],"skills":%s,"domains":%s,"created_at":"2026-01-01T00:00:00Z"}' "$2" "$3" "$4" "$5")
  cid="$(printf '%s' "$record" | "$DIRCTL_BIN" push --server-addr "$1" --stdin --output raw)"
  "$DIRCTL_BIN" routing publish "$cid" --server-addr "$1" >/dev/null
  echo "$1  $2  $cid"
}

case "$cmd" in
start)
  mkdir -p "$dir"
  start_node main localhost:8878 5545 8979 8879 ""
  main_id="$(grep -o 'peerID=12D3[[:alnum:]]*' "$dir/main.log" | tail -n 1 | cut -d= -f2)"
  [ -n "$main_id" ] || { echo "ERROR: main daemon peer ID not found in log" >&2; exit 1; }
  start_node peer localhost:8898 5565 9009 8899 "/ip4/127.0.0.1/tcp/8979/p2p/$main_id"
  ;;
seed)
  peer=localhost:8898
  seed_record $peer code-helper "Turns plain-language requests into code" \
    '[{"name":"software_engineering/code_generation/text_to_code"}]' \
    '[{"name":"technology/software_engineering"}]'
  seed_record $peer report-summarizer "Summarizes financial reports" \
    '[{"name":"language_processing/language_generation/summarization"},{"name":"language_processing/language_understanding/contextual_comprehension"}]' \
    '[{"name":"finance_and_business/finance"}]'
  seed_record $peer docs-rag "Answers questions over project docs" \
    '[{"name":"ai_ml_engineering/retrieval_augmented_generation"},{"name":"language_processing/language_generation/summarization"}]' \
    '[{"name":"technology/software_engineering"}]'
  seed_record $peer build-orchestrator "Coordinates coding agents" \
    '[{"name":"ai_ml_engineering/agent_orchestration"},{"name":"software_engineering/code_generation"}]' \
    '[{"name":"technology/software_engineering"}]'
  # Published on main, so routing search from main must not list it.
  seed_record localhost:8878 local-summarizer "Summarizes text on the main node" \
    '[{"name":"language_processing/language_generation/summarization"}]' '[]'
  ;;
stop)
  "$DIRCTL_BIN" daemon stop --data-dir "$dir/peer" >/dev/null 2>&1 || true
  "$DIRCTL_BIN" daemon stop --data-dir "$dir/main" >/dev/null 2>&1 || true
  ;;
*)
  echo "$usage" >&2
  exit 2
  ;;
esac
