# Go Parafia — intencje mszalne

Program do prowadzenia intencji mszalnych w parafii: tydzień po tygodniu, z gotowym wydrukiem na jedną kartkę A4.

Działa jako zwykły program dla Windows (`Intencje Mszalne.exe`), z jednym oknem, bez przeglądarki i bez instalacji. Dane trzyma lokalnie na komputerze.

## Co potrafi

- **Widok tygodnia** (poniedziałek–niedziela): msze, intencje parafialne i „poza parafią”, numeracja, szybkie dodawanie, edycja i usuwanie.
- **Porządki mszy**: przycisk „Wczytaj godziny mszy” tworzy puste miejsca na każdą mszę tygodnia według wybranego porządku (np. zwykły, wakacyjny). Porządki można edytować w ustawieniach.
- **Przypomnienie o mszach bez intencji** nad widokiem tygodnia.
- **Gregorianki i intencje za zmarłych**: checkboxy w formularzu, ozdobniki w widoku, a na wydruku „+” i „(greg.)”.
- **Święta i wspomnienia dnia**: wpisywane ręcznie albo pobierane przyciskiem „Pobierz święta z GCatholic” (wymaga internetu; rodzaje obchodów do wyboru w ustawieniach).
- **Wydruk PDF** całego tygodnia: nagłówek z nazwą i adresem parafii, cały tydzień dopasowany do jednej strony A4.
- **Wyczyść tydzień** (z potwierdzeniem).
- **Ustawienia** (ikona koła zębatego): nazwa i adres parafii, pobieranie świąt, porządki mszy.

## Uruchomienie u użytkownika (Windows)

1. Skopiuj `Intencje Mszalne.exe` w dowolne miejsce (np. na pulpit) i uruchom.
2. Windows 10/11 ma już potrzebny silnik okna (WebView2), więc nic nie trzeba doinstalowywać.
3. Program uruchamia się tylko raz. Ponowne kliknięcie pokazuje już otwarte okno.

Dane programu:

| Co | Gdzie |
|---|---|
| baza intencji, ustawień, porządków | `%AppData%\Go Parafia\data.db` |
| dziennik błędów | `%AppData%\Go Parafia\go-parafia.log` |
| dane silnika okna | `%AppData%\Go Parafia\webview\` |

**Kopia zapasowa** to po prostu kopia pliku `data.db` przy zamkniętym programie.

**„Zablokowaliśmy, ponieważ nie mogliśmy zidentyfikować wydawcy”**: program nie jest jeszcze podpisany cyfrowo, a Windows 11 z włączonym *Inteligentnym sterowaniem aplikacjami* (Smart App Control) może go zablokować. Rozwiązaniem docelowym jest podpis cyfrowy pliku.

## Dla programisty

Wymagania: **Go 1.26** (Linux albo WSL). Kompilator C nie jest potrzebny, bo SQLite jest w czystym Go (`github.com/glebarez/sqlite`).

```bash
make run       # wersja deweloperska: http://localhost:8080 (baza ./data.db)
make test      # gofmt + go vet (obie wersje) + testy, tak jak CI
make windows   # build/bin/Intencje Mszalne.exe (okno Wails, z ikoną i informacjami o wersji)
make clean
```

- Wersja deweloperska (`go run .`) to zwykły serwer HTTP na `127.0.0.1:8080`, a `.exe` to okno [Wails](https://wails.io) bez portu sieciowego. Wybór następuje przy kompilacji przez tagi (`server.go` vs `desktop.go` z `//go:build desktop`).
- Szablony, CSS, JS, czcionki PDF i domyślne porządki mszy są wkompilowane w program (`//go:embed`). Po ich zmianie trzeba uruchomić program od nowa.
- Numer wersji, ikona i opis pliku `.exe`: `build/windows/winres.json` (narzędzie `go-winres`, uruchamiane przez `make windows`).

### Struktura

```
main.go, routes.go          start programu, baza, router
server.go / desktop.go      serwer HTTP (dev) albo okno Wails (.exe)
internal/intentions         intencje: model, walidacja, zapytania tygodnia
internal/masses             porządki mszy: domyślne (schedules.json), baza, sloty
internal/calendar           opisy dni (święta, wspomnienia)
internal/gcatholic          pobieranie i odczyt kalendarza GCatholic (iCal)
internal/settings           ustawienia parafii
internal/web                strony HTML, formularze, wydruk PDF
internal/api                JSON API (/api/intentions)
```

### CI

GitHub Actions przy każdym pushu i pull requeście: formatowanie, `go vet` (także wersji Windows), `go mod tidy`, testy z `-race`, `govulncheck`, `golangci-lint`.

## Autor

Tomasz Kaliński, Gemma It Solutions
