# efs

Små hjälpfunktioner för att extrahera en katalog från ett inbäddat eller generellt `fs.FS` till en temporär katalog, med städning som hanterar avslutssignaler.

## Användning

```go
package main

import (
    "embed"
    "fmt"
    "log"
    "path/filepath"

    "github.com/skabbio1976/eFS"
)

//go:embed assets/**
var assets embed.FS

func main() {
    // Extrahera innehållet från katalogen "assets" i det inbäddade FS:et
    dir, cleanup, err := efs.ExtractToTemp(assets, "assets", "myassets")
    if err != nil { log.Fatal(err) }
    defer cleanup() // Säkerställ städning vid normal exit/panic

    stop := efs.StartCleanupListener(dir) // Städar även vid Ctrl+C/SIGTERM
    defer stop()

    fmt.Println("Tillfälliga filer extraherade till:", dir)
    fmt.Println("Exempel på fil:", filepath.Join(dir, "assets.css"))
}
```

## Beteende
- Om `root` är tom sträng används `"."` som rot.
- Den temporära katalogen innehåller själva innehållet i `root` (inte en extra rotmapp). Vill du ha en extra nivå, skapa den själv.
- Returnerar absolut sökväg till tempkatalogen när det går.
- `cleanup()` är idempotent och kan anropas flera gånger.
- `StartCleanupListener(dir)` returnerar en `stop()`-funktion för att avregistrera lyssnaren.

## Anteckningar
- `fs.FS` gör API:et generellt: funkar med `embed.FS`, `fstest.MapFS`, `os.DirFS`, `fs.Sub`, m.fl.
- Fil- och katalogrättigheter är 0644 respektive 0755 som standard.
