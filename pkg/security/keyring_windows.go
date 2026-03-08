//go:build windows

package security

// Windows Credential Manager Implementation
//
// Verwendet die Windows-API direkt via syscall (keine externe Abhängigkeit).
// Gespeichert als Generic Credentials in Advapi32.dll:
//   - CredReadW    → Secret lesen
//   - CredWriteW   → Secret schreiben
//   - CredDeleteW  → Secret löschen
//   - CredEnumerateW → alle Credentials mit Präfix auflisten
//   - CredFreeW    → allokierten Speicher freigeben
//
// Key-Format im Credential Manager: "FluxBot/KEY_NAME"
// → erscheint im Windows Credential Manager unter "Generische Anmeldedaten"

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var (
	advapi32        = syscall.NewLazyDLL("Advapi32.dll")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFreeW   = advapi32.NewProc("CredFreeW")
	procCredEnumW   = advapi32.NewProc("CredEnumerateW")
)

// Windows-Fehlercode: ERROR_NOT_FOUND
const errNotFound = syscall.Errno(1168)

// _CREDENTIAL spiegelt die Windows CREDENTIAL-Struktur.
// Felder-Reihenfolge und -Typen müssen exakt der Windows-API entsprechen.
// Referenz: https://learn.microsoft.com/en-us/windows/win32/api/wincred/ns-wincred-credentialw
type _CREDENTIAL struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     uintptr // *byte – uintptr vermeidet GC-Probleme mit unsafe.Pointer
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

const (
	_CRED_TYPE_GENERIC          = uint32(1)
	_CRED_PERSIST_LOCAL_MACHINE = uint32(2)
)

// targetName baut den vollständigen Credential-Namen: "FluxBot/KEY".
func targetName(service, key string) string {
	return service + "/" + key
}

// keyringGet liest einen Secret aus dem Windows Credential Manager.
func keyringGet(service, key string) (string, error) {
	target, err := syscall.UTF16PtrFromString(targetName(service, key))
	if err != nil {
		return "", fmt.Errorf("keyring get: ungültiger key: %w", err)
	}

	var pcred *_CREDENTIAL
	ret, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(_CRED_TYPE_GENERIC),
		0,
		uintptr(unsafe.Pointer(&pcred)),
	)
	if ret == 0 {
		if callErr == errNotFound {
			return "", errKeyringNotFound
		}
		return "", fmt.Errorf("CredReadW fehlgeschlagen: %w", callErr)
	}
	// CredFreeW ist auf manchen Windows-Versionen nicht per LazyProc auflösbar –
	// Find() prüft ohne Panic; bei Fehler wird der Speicher nicht freigegeben
	// (minimaler Leak, besser als Prozess-Crash).
	if err := procCredFreeW.Find(); err == nil {
		defer procCredFreeW.Call(uintptr(unsafe.Pointer(pcred)))
	}

	if pcred.CredentialBlobSize == 0 {
		return "", nil
	}
	// Blob als UTF-8 Bytes lesen (FluxBot speichert immer UTF-8)
	blob := unsafe.Slice((*byte)(unsafe.Pointer(pcred.CredentialBlob)), pcred.CredentialBlobSize)
	return string(blob), nil
}

// keyringSet schreibt einen Secret in den Windows Credential Manager.
func keyringSet(service, key, value string) error {
	targetStr := targetName(service, key)
	target, err := syscall.UTF16PtrFromString(targetStr)
	if err != nil {
		return fmt.Errorf("keyring set: ungültiger key: %w", err)
	}
	username, err := syscall.UTF16PtrFromString("fluxbot")
	if err != nil {
		return fmt.Errorf("keyring set: username encode: %w", err)
	}

	blob := []byte(value)
	cred := _CREDENTIAL{
		Type:               _CRED_TYPE_GENERIC,
		TargetName:         target,
		UserName:           username,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     uintptr(unsafe.Pointer(&blob[0])),
		Persist:            _CRED_PERSIST_LOCAL_MACHINE,
	}

	ret, _, callErr := procCredWriteW.Call(
		uintptr(unsafe.Pointer(&cred)),
		0,
	)
	if ret == 0 {
		return fmt.Errorf("CredWriteW fehlgeschlagen für '%s': %w", key, callErr)
	}
	return nil
}

// keyringDelete entfernt einen Secret aus dem Windows Credential Manager.
func keyringDelete(service, key string) error {
	target, err := syscall.UTF16PtrFromString(targetName(service, key))
	if err != nil {
		return fmt.Errorf("keyring delete: ungültiger key: %w", err)
	}
	ret, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(_CRED_TYPE_GENERIC),
		0,
	)
	if ret == 0 {
		if callErr == errNotFound {
			return errKeyringNotFound
		}
		return fmt.Errorf("CredDeleteW fehlgeschlagen für '%s': %w", key, callErr)
	}
	return nil
}

// keyringEnumDynamic listet alle Credentials mit dem Präfix "FluxBot/INTEG_" auf.
// Wird von KeyringProvider.GetAll() für dynamische INTEG_*-Keys verwendet.
func keyringEnumDynamic(service string) (map[string]string, error) {
	filter, err := syscall.UTF16PtrFromString(service + "/INTEG_*")
	if err != nil {
		return nil, err
	}

	var count uint32
	var pcreds uintptr // **_CREDENTIAL

	ret, _, callErr := procCredEnumW.Call(
		uintptr(unsafe.Pointer(filter)),
		0,
		uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&pcreds)),
	)
	if ret == 0 {
		if callErr == errNotFound {
			return map[string]string{}, nil // keine dynamischen Keys vorhanden
		}
		return nil, fmt.Errorf("CredEnumerateW fehlgeschlagen: %w", callErr)
	}
	if err := procCredFreeW.Find(); err == nil {
		defer procCredFreeW.Call(pcreds)
	}

	result := make(map[string]string, count)
	// pcreds ist ein Array von *_CREDENTIAL Zeigern
	credPtrs := unsafe.Slice((*uintptr)(unsafe.Pointer(pcreds)), count)
	prefix := service + "/"
	for _, ptr := range credPtrs {
		cred := (*_CREDENTIAL)(unsafe.Pointer(ptr))
		if cred == nil {
			continue
		}
		// TargetName zurück in Go-String umwandeln
		rawName := syscall.UTF16ToString(
			unsafe.Slice(cred.TargetName, 256),
		)
		// Präfix "FluxBot/" entfernen → Key-Name
		keyName := strings.TrimPrefix(rawName, prefix)
		if keyName == "" || keyName == rawName {
			continue
		}
		if cred.CredentialBlobSize == 0 {
			continue
		}
		blob := unsafe.Slice((*byte)(unsafe.Pointer(cred.CredentialBlob)), cred.CredentialBlobSize)
		result[keyName] = string(blob)
	}
	return result, nil
}

// keyringBackendName gibt den Anzeigenamen des Backends zurück.
func keyringBackendName() string {
	return "Windows Credential Manager"
}
