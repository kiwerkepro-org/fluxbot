// cmd/keyring-verify/main.go
//
// Verifikationsprogramm für die Keyring-Abstraktionsschicht.
// Testet: IsDockerEnvironment(), NewSecretProvider(), Vault-Read/Write.
//
// Verwendung (im Repo-Root):
//   go run ./cmd/keyring-verify --workspace ./workspace

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ki-werke/fluxbot/pkg/security"
)

func main() {
	workspace := flag.String("workspace", "./workspace", "Pfad zum workspace-Verzeichnis")
	flag.Parse()

	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║     FluxBot – Keyring-Verifikation               ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()

	// ── 1. Umgebungserkennung ──────────────────────────────────────────────────
	fmt.Println("── 1. Umgebungserkennung ────────────────────────────")
	isDocker := security.IsDockerEnvironment()
	fmt.Printf("   IsDockerEnvironment(): %v\n", isDocker)

	// Einzelne Erkennungsmethoden direkt prüfen
	_, dockerEnvErr := os.Stat("/.dockerenv")
	fmt.Printf("   /.dockerenv vorhanden:  %v\n", dockerEnvErr == nil)
	fmt.Printf("   FLUXBOT_DOCKER env:     %q\n", os.Getenv("FLUXBOT_DOCKER"))
	fmt.Printf("   DOCKER_ENV env:         %q\n", os.Getenv("DOCKER_ENV"))

	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		hasDocker := false
		for _, line := range []string{"docker", "kubepods"} {
			if len(content) > 0 {
				for i := 0; i+len(line) <= len(content); i++ {
					if content[i:i+len(line)] == line {
						hasDocker = true
						break
					}
				}
			}
		}
		fmt.Printf("   /proc/1/cgroup:         lesbar, docker/kubepods=%v\n", hasDocker)
	} else {
		fmt.Printf("   /proc/1/cgroup:         nicht lesbar (%v)\n", err)
	}

	if isDocker {
		fmt.Println("   → Modus: DOCKER (Vault wird verwendet)")
	} else {
		fmt.Println("   → Modus: LOKAL (Keyring → Vault Fallback)")
	}
	fmt.Println()

	// ── 2. SecretProvider Factory ──────────────────────────────────────────────
	fmt.Println("── 2. NewSecretProvider() Factory ───────────────────")
	provider, err := security.NewSecretProvider(*workspace)
	if err != nil {
		log.Fatalf("   ❌ FEHLER: %v", err)
	}
	fmt.Printf("   ✅ Provider initialisiert\n")
	fmt.Printf("   Backend: %q\n", provider.Backend())
	fmt.Println()

	// ── 3. Vault lesen ────────────────────────────────────────────────────────
	fmt.Println("── 3. Vault lesen (GetAll) ──────────────────────────")
	all, err := provider.GetAll()
	if err != nil {
		fmt.Printf("   ❌ GetAll Fehler: %v\n", err)
	} else {
		fmt.Printf("   ✅ %d Secret-Keys im Vault\n", len(all))
		// Zeige Keys (KEINE Werte!) für Verifikation
		keyGroups := map[string][]string{
			"Kanäle":    {},
			"Provider":  {},
			"System":    {},
			"Google":    {},
			"Cal.com":   {},
		}
		for k := range all {
			switch {
			case k == "TELEGRAM_TOKEN" || k == "DISCORD_TOKEN" || k == "SLACK_BOT_TOKEN":
				keyGroups["Kanäle"] = append(keyGroups["Kanäle"], k)
			case len(k) > 9 && k[:9] == "PROVIDER_":
				keyGroups["Provider"] = append(keyGroups["Provider"], k)
			case k == "SKILL_SECRET" || k == "DASHBOARD_PASSWORD" || k == "HMAC_SECRET" || k == "VIRUSTOTAL_API_KEY":
				keyGroups["System"] = append(keyGroups["System"], k)
			case len(k) > 7 && k[:7] == "GOOGLE_":
				keyGroups["Google"] = append(keyGroups["Google"], k)
			case len(k) > 6 && k[:6] == "CALCOM":
				keyGroups["Cal.com"] = append(keyGroups["Cal.com"], k)
			}
		}
		for group, keys := range keyGroups {
			if len(keys) > 0 {
				fmt.Printf("   %-10s %v\n", group+":", keys)
			}
		}
	}
	fmt.Println()

	// ── 4. Einzelnen Key lesen ────────────────────────────────────────────────
	fmt.Println("── 4. Einzelnen Key lesen (Get) ─────────────────────")
	for _, key := range []string{"SKILL_SECRET", "DASHBOARD_PASSWORD", "HMAC_SECRET"} {
		val, err := provider.Get(key)
		if err != nil {
			fmt.Printf("   %-25s ❌ Fehler: %v\n", key+":", err)
		} else if val == "" {
			fmt.Printf("   %-25s ⚠️  leer / nicht gesetzt\n", key+":")
		} else {
			// Nur Länge und erste 4 Zeichen zeigen (sicher)
			preview := val[:min(4, len(val))] + "****"
			fmt.Printf("   %-25s ✅ gesetzt (%d Zeichen, beginnt mit: %s)\n", key+":", len(val), preview)
		}
	}
	fmt.Println()

	// ── 5. Test-Key schreiben + lesen + löschen ───────────────────────────────
	fmt.Println("── 5. Write/Read/Delete Test ────────────────────────")
	testKey := "__fluxbot_keyring_test__"
	testVal := "keyring-test-wert-12345"

	// Schreiben
	if err := provider.Set(testKey, testVal); err != nil {
		fmt.Printf("   Set:    ❌ %v\n", err)
	} else {
		fmt.Printf("   Set:    ✅ Wert gespeichert\n")
	}

	// Lesen zurück
	readVal, err := provider.Get(testKey)
	if err != nil {
		fmt.Printf("   Get:    ❌ %v\n", err)
	} else if readVal == testVal {
		fmt.Printf("   Get:    ✅ Wert korrekt gelesen\n")
	} else {
		fmt.Printf("   Get:    ❌ Wert falsch (erwartet: %q, erhalten: %q)\n", testVal, readVal)
	}

	// Löschen
	if err := provider.Delete(testKey); err != nil {
		fmt.Printf("   Delete: ❌ %v\n", err)
	} else {
		// Prüfen ob wirklich weg
		afterDelete, _ := provider.Get(testKey)
		if afterDelete == "" {
			fmt.Printf("   Delete: ✅ Key erfolgreich entfernt\n")
		} else {
			fmt.Printf("   Delete: ⚠️  Key noch vorhanden nach Delete\n")
		}
	}
	fmt.Println()

	// ── Zusammenfassung ───────────────────────────────────────────────────────
	fmt.Println("── Zusammenfassung ──────────────────────────────────")
	fmt.Printf("   Backend:  %s\n", provider.Backend())
	fmt.Printf("   Docker:   %v\n", isDocker)
	if isDocker {
		fmt.Println("   ✅ Docker-Modus korrekt erkannt → Vault aktiv")
	} else {
		fmt.Println("   ✅ Lokaler Modus erkannt → Keyring/Vault aktiv")
	}
	fmt.Println()
	fmt.Println("Verifikation abgeschlossen.")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
