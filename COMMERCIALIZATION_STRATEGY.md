# Commercialization & Security Strategy

**Date:** June 21, 2026

This document outlines the long-term vision and architectural requirements for commercializing OpenProphet as a standalone, autonomous trading product.

---

## 1. Architectural Model: Licensed Standalone Product

When commercializing an autonomous trading bot, a **Multi-Tenant (SaaS) Architecture** should be strictly avoided due to severe liability, regulatory red tape, and the technical nightmare of managing hundreds of concurrent IBKR Gateway connections and storing users' brokerage credentials.

Instead, OpenProphet will be distributed as a **Licensed Standalone Product**.

### Benefits of the Standalone Model:
* **Zero Liability:** The software runs on the user's hardware (Desktop or personal VPS), utilizing their own LLM API keys and executing on their local IBKR Gateway. If their network fails or a trade goes against them, the liability remains with the user.
* **Privacy & Security:** OpenProphet never centralizes or touches user funds, IBKR passwords, or LLM billing accounts.
* **Regulatory Shield:** The product is sold as a software tool, avoiding classification as a registered broker or investment advisor.

---

## 2. Licensing Enforcement

To enforce time-bound subscriptions (e.g., yearly licenses) without a multi-tenant backend, we will utilize a minimal **License Verification Server**:

1. **The Cloud Component:** A lightweight cloud server dedicated solely to issuing and verifying cryptographic license keys.
2. **Phone-Home Mechanism:** Upon boot (and periodically, e.g., every 24 hours), the OpenProphet backend "phones home" with the user's License Key and Machine ID.
3. **The Kill Switch:** If the server returns an invalid or expired status, the Go backend immediately disables all trading routes (`/api/v1/trade`, etc.) and locks the UI until a valid subscription is detected.

---

## 3. Securing the System: The "Zero-Trust" UI

Because JavaScript is interpreted and runs on the client machine, it is mathematically impossible to make the UI "bulletproof." A determined user can always deobfuscate and modify frontend code to bypass a JS-based license check.

Therefore, we shift the trust boundary entirely to the compiled Go backend.

### The Defense Strategy:
1. **The "Dumb" UI:** The JavaScript frontend is strictly responsible for rendering views. It never handles license verification, nor does it communicate directly with IBKR or the LLM APIs.
2. **The Go Fortress:** All critical logic, API keys, trading algorithms, and license checks live exclusively inside the compiled Go backend. Decompiling Go machine code into readable source is exponentially more difficult than tampering with JavaScript.
3. **Cryptographic Token Handshakes:** 
   - Upon successful license verification, the Go backend generates a cryptographically signed, short-lived JWT (JSON Web Token) and passes it to the UI.
   - Every execution request from the UI to the Go backend must include this token.
   - If the UI is hacked to display "License Valid", any trade execution attempt will still be rejected by the backend due to a missing or invalid token.
4. **Embedded Binaries (The Ultimate Deterrent):** For commercial deployment, the Node.js server and raw JS files will be eliminated. The entire React/Vue frontend will be compiled into static assets and baked directly into the Go binary using Go's native `//go:embed` feature. The Go executable will serve the UI from its own memory, leaving no raw JavaScript files on the user's hard drive to tamper with.
5. **Standard Obfuscation:** The embedded JavaScript will still undergo aggressive minification and obfuscation (e.g., Terser, JavaScript-Obfuscator) as a speed bump to deter casual reverse engineering.
