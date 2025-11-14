# efs

Små hjälpfunktioner för att extrahera kataloger eller enskilda filer från ett inbäddat eller generellt `fs.FS` till temporära filer/kataloger, med städning som hanterar avslutssignaler.

## Temp-katalog strategi

**Viktigt att förstå:**
- Varje anrop till `ExtractToTemp()` eller `ExtractFile()` skapar en **NY** temporär katalog/fil.
- Om du anropar `ExtractToTemp()` 100 gånger kommer 100 separata temp-kataloger att skapas.
- Varje temp-katalog/fil får ett unikt namn baserat på prefixet och en slumpmässig suffix.
- Det är anroparens ansvar att anropa `cleanup()` för att ta bort temp-kataloger/filer.
- Använd `StartCleanupListener()` för att automatiskt städa vid programavslut (Ctrl+C/SIGTERM).
- Som standard skapas temp-kataloger i den aktuella arbetskatalogen.
- Du kan ange en anpassad baskatalog med `tempDir`-parametern (tom sträng = standard).

## Användning

### Exempel 1: Extrahera en katalog

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
    // Tom sträng som sista parameter = använd aktuell arbetskatalog
    dir, cleanup, err := efs.ExtractToTemp(assets, "assets", "myassets", "")
    if err != nil { log.Fatal(err) }
    defer cleanup() // Säkerställ städning vid normal exit/panic

    stop := efs.StartCleanupListener(dir) // Städar även vid Ctrl+C/SIGTERM
    defer stop()

    fmt.Println("Tillfälliga filer extraherade till:", dir)
    fmt.Println("Exempel på fil:", filepath.Join(dir, "assets.css"))
}
```

### Exempel 2: Extrahera en enskild fil

```go
package main

import (
    "embed"
    "fmt"
    "log"
    "os"

    "github.com/skabbio1976/eFS"
)

//go:embed config.json
var config embed.FS

func main() {
    // Extrahera en enskild fil
    file, cleanup, err := efs.ExtractFile(config, "config.json", "config", "")
    if err != nil { log.Fatal(err) }
    defer cleanup()

    // Läsa filen
    data, err := os.ReadFile(file)
    if err != nil { log.Fatal(err) }
    fmt.Println("Config innehåll:", string(data))
}
```

### Exempel 3: Anpassad temp-katalog

```go
package main

import (
    "embed"
    "log"
    "os"
    "path/filepath"
    "strings"

    "github.com/skabbio1976/eFS"
)

//go:embed assets/**
var assets embed.FS

func main() {
    // Skapa en anpassad temp-katalog (t.ex. i /tmp)
    customTempDir := "/tmp/myapp"
    if err := os.MkdirAll(customTempDir, 0755); err != nil {
        log.Fatal(err)
    }

    // Extrahera till den anpassade katalogen
    dir, cleanup, err := efs.ExtractToTemp(assets, "assets", "myassets", customTempDir)
    if err != nil { log.Fatal(err) }
    defer cleanup()

    // Verifiera att katalogen är i rätt plats
    rel, err := filepath.Rel(customTempDir, dir)
    if err != nil || strings.HasPrefix(rel, "..") {
        log.Fatal("Katalogen är inte i rätt plats!")
    }
}
```

### Exempel 4: Flera extraktioner

```go
package main

import (
    "embed"
    "fmt"
    "log"

    "github.com/skabbio1976/eFS"
)

//go:embed assets/**
var assets embed.FS

func main() {
    // Varje anrop skapar en ny temp-katalog
    dir1, cleanup1, err := efs.ExtractToTemp(assets, "assets", "extract1", "")
    if err != nil { log.Fatal(err) }
    defer cleanup1()

    dir2, cleanup2, err := efs.ExtractToTemp(assets, "assets", "extract2", "")
    if err != nil { log.Fatal(err) }
    defer cleanup2()

    fmt.Printf("Första extraktionen: %s\n", dir1)
    fmt.Printf("Andra extraktionen: %s\n", dir2)
    // OBS: dir1 och dir2 är olika kataloger!
}
```

### Exempel 5: Använda med os.DirFS

```go
package main

import (
    "fmt"
    "io/fs"
    "log"
    "os"

    "github.com/skabbio1976/eFS"
)

func main() {
    // Använd ett riktigt filsystem
    fsys := os.DirFS("/path/to/source")

    // Extrahera en fil
    file, cleanup, err := efs.ExtractFile(fsys, "config.json", "config", "")
    if err != nil { log.Fatal(err) }
    defer cleanup()

    fmt.Println("Extraherad fil:", file)
}
```

## API

### ExtractToTemp

```go
func ExtractToTemp(fsys fs.FS, root string, tempPrefix string, tempDir string) (string, func(), error)
```

Extraherar innehållet från en katalog i `fsys` till en temporär katalog.

**Parametrar:**
- `fsys`: Filsystemet att extrahera från (embed.FS, fstest.MapFS, os.DirFS, etc.)
- `root`: Rot-sökvägen inom fsys att extrahera (tom sträng = ".")
- `tempPrefix`: Prefix för temp-katalogens namn
- `tempDir`: Baskatalog där temp-katalogen skapas (tom sträng = aktuell arbetskatalog)

**Returvärden:**
- Absolut sökväg till temp-katalogen
- Idempotent cleanup-funktion
- Eventuellt fel

### ExtractFile

```go
func ExtractFile(fsys fs.FS, filePath string, tempPrefix string, tempDir string) (string, func(), error)
```

Extraherar en enskild fil från `fsys` till en temporär fil.

**Parametrar:**
- `fsys`: Filsystemet att extrahera från
- `filePath`: Sökvägen till filen inom fsys
- `tempPrefix`: Prefix för temp-filens namn
- `tempDir`: Baskatalog där temp-filen skapas (tom sträng = aktuell arbetskatalog)

**Returvärden:**
- Absolut sökväg till temp-filen
- Idempotent cleanup-funktion
- Eventuellt fel

**Notera:** Filens ursprungliga extension bevaras i temp-filnamnet.

### StartCleanupListener

```go
func StartCleanupListener(dir string) (stop func())
```

Startar en goroutine som lyssnar på avslutssignaler (SIGINT, SIGTERM, SIGHUP) och städar den angivna katalogen innan programmet avslutas.

## Beteende
- Om `root` är tom sträng används `"."` som rot.
- Den temporära katalogen innehåller själva innehållet i `root` (inte en extra rotmapp). Vill du ha en extra nivå, skapa den själv.
- Returnerar absolut sökväg till tempkatalogen när det går.
- `cleanup()` är idempotent och kan anropas flera gånger.
- `StartCleanupListener(dir)` returnerar en `stop()`-funktion för att avregistrera lyssnaren.

## Anteckningar
- `fs.FS` gör API:et generellt: funkar med `embed.FS`, `fstest.MapFS`, `os.DirFS`, `fs.Sub`, m.fl.
- Fil- och katalogrättigheter är 0644 respektive 0755 som standard.
- `ExtractToTemp()` och `ExtractFile()` är thread-safe och kan anropas concurrent från flera goroutines.
- `cleanup()` är idempotent och thread-safe (använder `sync.Once` internt).
- Varje anrop skapar en ny temp-katalog/fil - kom ihåg att städa upp!
