#!/usr/bin/env bash
# =============================================================================
# OpenSIN Safe Process Bootstrapper & Environment Injector
# =============================================================================
# Behebt den {env:...}-Bug in opencode.jsonc durch native Vorbelegung.
# Dieser Wrapper:
#   1. Liest die opencode.jsonc
#   2. Parst Umgebungsvariablen nativ via Node.js
#   3. Ersetzt {env:VAR} mit tatsächlichen Werten (mit $-Zeichen-Schutz)
#   4. Startet OpenCode mit der gepatchten Konfiguration
# 
# Usage: ./opencode-safe-start.sh [opencode_args...]
# 
# Docs: opencode-safe-start.doc.md
# =============================================================================

set -euo pipefail

# ── Konfiguration ──────────────────────────────────────────────────────────
CONFIG_PATH="${OPENCODE_CONFIG:-${HOME}/.config/opencode/opencode.jsonc}"
TEMP_DIR="${TMPDIR:-/tmp}"
TEMP_CONFIG="${TEMP_DIR}/opencode.running.$(date +%s).jsonc"

# ── Hilfsfunktionen ───────────────────────────────────────────────────────
log_info() {
    echo "ℹ️  $1" >&2
}

log_warn() {
    echo "⚠️  $1" >&2
}

log_error() {
    echo "❌ $1" >&2
}

log_success() {
    echo "✅ $1" >&2
}

# ── Validierung ───────────────────────────────────────────────────────────
if [[ ! -f "$CONFIG_PATH" ]]; then
    log_error "opencode.jsonc nicht gefunden: $CONFIG_PATH"
    log_info "Setze OPENCODE_CONFIG um den Pfad zu ändern"
    exit 1
fi

log_info "Lade Konfiguration: $CONFIG_PATH"

# ── Umgebungsvariablen-Validierung ────────────────────────────────────────
validate_env() {
    local var_name="$1"
    local var_value="${!var_name:-}"
    
    if [[ -z "$var_value" ]]; then
        log_warn "Variable $var_name ist in der Shell nicht definiert"
        return 1
    fi
    
    # Warnung bei $-Zeichen (könnten Probleme machen)
    if [[ "$var_value" == *\$* ]]; then
        log_warn "Variable $var_name enthält $-Zeichen - wir werden es escapen"
    fi
    
    return 0
}

# Prüfe kritische API-Keys
for env_var in TAVILY_API_KEY NVIDIA_API_KEY ANTHROPIC_API_KEY OPENAI_API_KEY; do
    if [[ -n "${!env_var:-}" ]]; then
        log_info "✓ $env_var ist gesetzt"
    else
        log_warn "$env_var ist nicht gesetzt"
    fi
done

# ── Node.js Parser ────────────────────────────────────────────────────────
# Verwende Node.js, um die JSONC zu parsen und {env:VAR} zu ersetzen
# Dies umgeht die fehlerhafte Substitution in OpenCode

log_info "Bereite Laufzeit-Konfiguration vor..."

node -e '
const fs = require("fs");
const path = process.argv[1];
const outputPath = process.argv[2];

let content = fs.readFileSync(path, "utf8");

// Entferne JSONC-Kommentare (// und /* */)
content = content.replace(/\/\/.*$/gm, "");
content = content.replace(/\/\*[\s\S]*?\*\//g, "");

// Regex sucht nach {env:VARIABLEN_NAME}
const envRegex = /"\{env:([A-Za-z0-9_]+)\}"/g;

let match;
const envMap = {};
while ((match = envRegex.exec(content)) !== null) {
    envMap[match[1]] = true;
}

// Ersetze alle gefundenen {env:VAR} mit tatsächlichen Werten
content = content.replace(envRegex, (match, envVar) => {
    const val = process.env[envVar];
    if (!val) {
        console.warn(`⚠️  Variable ${envVar} ist in der Shell nicht definiert.`);
        return `""`; // Leerer String als Fallback
    }
    
    // WICHTIG: Escaped Sonderzeichen wie $, \ und " für sicheres JSON
    // Das ist der kritische Fix für das $-Zeichen-Problem!
    const escaped = val
        .replace(/\\/g, "\\\\")   // Backslash zuerst!
        .replace(/\"/g, "\\\"")
        .replace(/\$/g, "\\$");    // Dollar-Zeichen escapen
    
    return `"${escaped}"`;
});

// Schreibe gepatchte Konfiguration
fs.writeFileSync(outputPath, content, "utf8");

console.log(`✅ Konfiguration geschrieben: ${outputPath}`);
console.log(`   Ersetzte Variablen: ${Object.keys(envMap).join(", ")}`);
' "$CONFIG_PATH" "$TEMP_CONFIG"

# ── Start ─────────────────────────────────────────────────────────────────
log_success "OpenCode-Start vorbereitet"
log_info "Temporäre Konfiguration: $TEMP_CONFIG"

# Führe opencode mit der temporären gepatchten Konfig aus
# Setze die Konfiguration als Umgebungsvariable
export OPENCODE_CONFIG="$TEMP_CONFIG"

# Cleanup-Funktion für Exit-Trap
cleanup() {
    if [[ -f "$TEMP_CONFIG" ]]; then
        rm -f "$TEMP_CONFIG"
        log_info "Temporäre Konfiguration aufgeräumt"
    fi
}

# Registriere Cleanup-Trap
trap cleanup EXIT INT TERM

log_info "Starte OpenCode..."
exec opencode "$@"
