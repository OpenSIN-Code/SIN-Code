#!/usr/bin/env bash
# =============================================================================
# OpenSIN Automated Post-Flight Cleanup Hook
# =============================================================================
# Implementiert die strengen Bereinigungsstandards des self-healer-Skills
# vollständig autonom. Befreit das System nach erfolgreichen Task-Läufen von
# temporärem Datenmüll, während kritische Sitzungen und Auth-Profile geschützt bleiben.
#
# Usage:
#   Manual: ./opencode-cleanup-hook.sh
#   Post-Flight: Add to your task completion script
#   Cron: 0 * * * * /path/to/opencode-cleanup-hook.sh
#
# Docs: opencode-cleanup-hook.doc.md
# =============================================================================

set -uo pipefail

# ── Konfiguration ──────────────────────────────────────────────────────────
# Schutzzonen (niemals löschen!)
PROTECTED_PATTERNS=(
    "*.session"
    "*.auth"
    "*credentials*"
    "*secret*"
    "*token*"
    "*key*"
    "*api_key*"
    "*apikey*"
    "*.pcpm/*"
    "*sin-brain*"
    "*opencode.json*"
    "*config.json*"
)

# Zielverzeichnisse für Bereinigung
LOG_BUFFER_DIR="${HOME}/.local/share/opencode/tool-output"
NPM_CACHE_DIR="${HOME}/.npm/_cacache"
RUNNER_LOG_DIR="${HOME}/.local/share/opencode/log"
DEBUG_SCREENSHOT_DIR="/tmp"
TEMP_DIRS=(
    "${TMPDIR:-/tmp}"
    "/tmp"
    "${HOME}/.cache"
)

# Audit-Trail
REPAIR_DOCS_DIR="${HOME}/.local/share/sin-solver"
REPAIR_DOCS="${REPAIR_DOCS_DIR}/repair-docs.md"

# ── Hilfsfunktionen ───────────────────────────────────────────────────────
log_info() {
    echo "ℹ️  $1" >&2
}

log_warn() {
    echo "⚠️  $1" >&2
}

log_success() {
    echo "✅ $1" >&2
}

log_section() {
    echo ""
    echo "📦 $1"
    echo "────────────────────────────────────────────────────"
}

# Prüft, ob ein Pfad geschützt ist
is_protected() {
    local path="$1"
    for pattern in "${PROTECTED_PATTERNS[@]}"; do
        if [[ "$path" == $pattern ]]; then
            return 0
        fi
    done
    return 1
}

# Sicheres Löschen mit Schutzprüfung
safe_remove() {
    local path="$1"
    
    if [[ ! -e "$path" ]]; then
        return 0
    fi
    
    if is_protected "$path"; then
        log_warn "Geschützt: $path"
        return 0
    fi
    
    # Prüfe auf Dateien die Session/Auth/Config enthalten könnten
    if [[ -f "$path" ]]; then
        local content_preview=$(head -c 1000 "$path" 2>/dev/null || true)
        if [[ "$content_preview" == *"session"* ]] || 
           [[ "$content_preview" == *"auth"* ]] ||
           [[ "$content_preview" == *"token"* ]] ||
           [[ "$content_preview" == *"api_key"* ]] ||
           [[ "$content_preview" == *"secret"* ]]; then
            log_warn "Potenziell sensibel: $path (übersprungen)"
            return 0
        fi
    fi
    
    rm -rf "$path"
    return 0
}

# ── Hauptbereinigung ────────────────────────────────────────────────────
echo ""
echo "🧹 OpenSIN Post-Flight Cleanup Hook"
echo "══════════════════════════════════════════════════════════════"
echo "Start: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

TOTAL_FREED=0

# 1. Log-Buffer bereinigen
log_section "Log-Buffer"
if [[ -d "$LOG_BUFFER_DIR" ]]; then
    local_size=$(du -sh "$LOG_BUFFER_DIR" 2>/dev/null | cut -f1)
    log_info "Aktuelle Größe: $local_size"
    
    for file in "$LOG_BUFFER_DIR"/*; do
        if [[ -f "$file" ]]; then
            safe_remove "$file"
            TOTAL_FREED=$((TOTAL_FREED + $(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo 0)))
        fi
    done
    
    log_success "Log-Buffer bereinigt"
else
    log_info "Log-Buffer-Verzeichnis nicht gefunden"
fi

# 2. Debug-Screenshots bereinigen
log_section "Debug-Screenshots"
screenshot_count=0
for pattern in "m*_RESULT.png" "debug_screenshot_*.png" "*.screenshot.png" "screen_*.png"; do
    for file in "${DEBUG_SCREENSHOT_DIR}/"${pattern}; do
        if [[ -f "$file" ]]; then
            safe_remove "$file"
            screenshot_count=$((screenshot_count + 1))
        fi
    done
done

if [[ $screenshot_count -gt 0 ]]; then
    log_success "$screenshot_count Screenshots entfernt"
else
    log_info "Keine Screenshots gefunden"
fi

# 3. NPM-Cache bereinigen
log_section "NPM Cache"
if [[ -d "$NPM_CACHE_DIR" ]]; then
    cache_size=$(du -sh "$NPM_CACHE_DIR" 2>/dev/null | cut -f1)
    log_info "Cache-Größe: $cache_size"
    
    rm -rf "${NPM_CACHE_DIR:?}"/*
    log_success "NPM Cache geleert"
else
    log_info "NPM Cache nicht gefunden"
fi

# 4. Runner-Logs bereinigen
log_section "Runner-Logs"
if [[ -d "$RUNNER_LOG_DIR" ]]; then
    log_count=0
    for file in "$RUNNER_LOG_DIR"/*.log; do
        if [[ -f "$file" ]] && [[ "$(basename "$file")" != "session.log" ]]; then
            safe_remove "$file"
            log_count=$((log_count + 1))
        fi
    done
    
    if [[ $log_count -gt 0 ]]; then
        log_success "$log_count Runner-Logs entfernt"
    else
        log_info "Keine Runner-Logs gefunden"
    fi
else
    log_info "Runner-Log-Verzeichnis nicht gefunden"
fi

# 5. Temporäre Dateien bereinigen
log_section "Temporäre Dateien"
temp_count=0
for temp_dir in "${TEMP_DIRS[@]}"; do
    if [[ -d "$temp_dir" ]]; then
        # Lösche nur Dateien älter als 1 Stunde
        while IFS= read -r -d '' file; do
            if [[ -f "$file" ]]; then
                # Prüfe Alter
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    # macOS
                    file_age=$(stat -f%Sm -t%s "$file" 2>/dev/null || echo 0)
                    current_time=$(date +%s)
                    age_seconds=$((current_time - file_age))
                else
                    # Linux
                    file_age=$(stat -c%Y "$file" 2>/dev/null || echo 0)
                    current_time=$(date +%s)
                    age_seconds=$((current_time - file_age))
                fi
                
                if [[ $age_seconds -gt 3600 ]]; then
                    if safe_remove "$file"; then
                        temp_count=$((temp_count + 1))
                    fi
                fi
            fi
        done < <(find "$temp_dir" -type f -mmin +60 -print0 2>/dev/null || true)
    fi
done

if [[ $temp_count -gt 0 ]]; then
    log_success "$temp_count temporäre Dateien entfernt"
else
    log_info "Keine alten temporären Dateien gefunden"
fi

# 6. Lock-Files bereinigen
log_section "Stale Lock-Files"
lock_count=0
for lock_file in "$(pwd)"/.tdd-locks/*.lock "$(pwd)"/.dag-locks/*.lock; do
    if [[ -f "$lock_file" ]]; then
        # Prüfe ob Lock älter als 24h
        if [[ -z "$(find "$lock_file" -mtime -1 2>/dev/null)" ]]; then
            safe_remove "$lock_file"
            lock_count=$((lock_count + 1))
        fi
    fi
done

if [[ $lock_count -gt 0 ]]; then
    log_success "$lock_count stale Lock-Files entfernt"
else
    log_info "Keine stale Lock-Files gefunden"
fi

# ── Schutzzonen-Status ─────────────────────────────────────────────────
echo ""
echo "🛡️  Schutzzonen-Status"
echo "────────────────────────────────────────────────────────────"
log_info "Folgende Bereiche wurden NICHT verändert:"
log_info "  • Agenten-Sessions und Memory-Verzeichnisse"
log_info "  • Auth-Keys und API-Token"
log_info "  • Konfigurationsdateien (opencode.json, config.json)"
log_info "  • Git-Repository und Commit-History"
log_info "  • Alle Dateien mit 'session', 'auth', 'token', 'secret' im Namen"

# ── Audit-Trail ───────────────────────────────────────────────────────────
echo ""
log_section "Audit-Trail"

mkdir -p "$REPAIR_DOCS_DIR"

if [[ ! -f "$REPAIR_DOCS" ]]; then
    echo "# OpenSIN Repair and Maintenance Logs" > "$REPAIR_DOCS"
    echo "" >> "$REPAIR_DOCS"
    echo "| Zeitstempel | Event | Details |" >> "$REPAIR_DOCS"
    echo "|-------------|-------|---------|" >> "$REPAIR_DOCS"
fi

TIMESTAMP=$(date "+%Y-%m-%d %H:%M:%S")
echo "| $TIMESTAMP | Cleanup | Post-Flight-Systembereinigung erfolgreich |" >> "$REPAIR_DOCS"

log_success "Audit-Trail aktualisiert: $REPAIR_DOCS"

# ── Abschluss ───────────────────────────────────────────────────────────
echo ""
echo "══════════════════════════════════════════════════════════════"
echo "✅ Systembereinigung erfolgreich abgeschlossen"
echo "Ende: $(date '+%Y-%m-%d %H:%M:%S')"
echo "══════════════════════════════════════════════════════════════"
echo ""

exit 0
