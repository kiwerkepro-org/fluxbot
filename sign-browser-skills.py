#!/usr/bin/env python3
"""
sign-browser-skills.py
Signiert alle neuen Browser-Skills für FluxBot (Session 42).

Verwendung:
  python3 sign-browser-skills.py <SKILL_SECRET>

Den SKILL_SECRET findest du im Dashboard unter Secrets → SKILL_SECRET
oder im Vault.
"""

import hmac
import hashlib
import sys
import os

def sign_skill(path: str, secret: str) -> str:
    with open(path, "rb") as f:
        data = f.read()
    sig = hmac.new(secret.encode(), data, hashlib.sha256).hexdigest()
    sig_path = path + ".sig"
    with open(sig_path, "w") as f:
        f.write(sig)
    return sig_path

def main():
    if len(sys.argv) < 2:
        print("Verwendung: python3 sign-browser-skills.py <SKILL_SECRET>")
        print()
        print("Den SKILL_SECRET findest du im Dashboard unter Secrets → SKILL_SECRET")
        sys.exit(1)

    secret = sys.argv[1]

    # Skill-Dateien die signiert werden sollen
    skills_dir = os.path.join(os.path.dirname(__file__), "workspace", "skills")
    new_skills = [
        "web-search.md",
        "browser-read.md",
        "browser-screenshot.md",
        "browser-fill.md",
    ]

    print("🔐 Signiere Browser-Skills...")
    print()

    success = 0
    for skill in new_skills:
        path = os.path.join(skills_dir, skill)
        if not os.path.exists(path):
            print(f"  ⚠️  Nicht gefunden: {skill}")
            continue
        sig_path = sign_skill(path, secret)
        print(f"  ✅ {skill} → {os.path.basename(sig_path)}")
        success += 1

    print()
    print(f"✅ {success}/{len(new_skills)} Skills signiert.")
    print()
    print("Nächste Schritte:")
    print("  1. docker compose down && docker compose up -d --build")
    print("  2. Im Dashboard unter Secrets folgende Keys eintragen:")
    print("     - SEARCH_API_KEY  → Tavily API-Key (https://tavily.com)")
    print("     - BROWSER_ENDPOINT → ws://localhost:9222")
    print("     - BROWSER_ALLOWED_DOMAINS → kommagetrennte Domains (z.B. 'google.com,ki-werke.at')")

if __name__ == "__main__":
    main()
