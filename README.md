# Go Parafia — intencje mszalne

Program do prowadzenia intencji mszalnych w parafii: tydzień po tygodniu, z gotowym wydrukiem A4.

Działa jako zwykły program dla Windows (`Intencje Mszalne.exe`), z jednym oknem, bez przeglądarki i bez instalacji. Dane trzyma lokalnie na komputerze.

## Co potrafi

- **Widok tygodnia** (poniedziałek–niedziela): msze, intencje parafialne i „poza parafią”, numeracja, szybkie dodawanie, edycja i usuwanie.
- **Podsumowanie dnia** w nagłówku każdego dnia, np. „2 Msze · 3 intencje · 1 wolna (+1 poza parafią)” albo „wszystkie zajęte”. Intencja „poza parafią” jest odprawiana gdzie indziej, więc nie zajmuje Mszy w parafii.
- **Szukaj Mszy** (lupa w pasku tygodnia): filtry jak w sklepie internetowym, z licznikami:
  - okres: miesiąc, 3 miesiące, pół roku, rok,
  - liczba intencji na Mszy: wolne, 1, 2, 3+,
  - dzień tygodnia i godzina,
  - pomijanie Mszy z gregorianką.

  Po zaznaczeniu „Pokaż Msze planowane” wyszukiwarka dokłada dla tygodni bez wczytanych godzin Msze „planowane” z wybranego porządku. Wyniki można oglądać jako **listę dni** (kliknięcie Mszy otwiera formularz z ustawionym dniem i godziną) albo jako **kalendarz**:
  - każda Msza dnia to kreska,
  - dni pasujące do filtrów są obramowane,
  - po kliknięciu dnia widać jego Msze z przyciskami „+ Dodaj”.
- **Kalendarz roczny**: wybór tygodnia z podglądem liczby intencji i dni z Mszami do uzupełnienia. Po najechaniu na dzień (także w kalendarzu wyszukiwarki) pojawia się podpowiedź z Mszami tego dnia, np. „2 Msze · 3 intencje · 1 wolna (+1 poza parafią)”.
- **Porządki mszy**: przycisk „Wczytaj godziny mszy” tworzy puste miejsca na każdą mszę tygodnia według wybranego porządku (np. zwykły, wakacyjny). Porządki można edytować w ustawieniach.
- **Przypomnienie o mszach bez intencji** nad widokiem tygodnia.
- **Intencje za zmarłych** (na wydruku „+”) i **Msze gregoriańskie**:
  - pierwsza gregorianka otwiera serię 30 Mszy,
  - kolejne Msze serii wybiera się w formularzu albo z przypomnienia przy dniu, w którym przypada następna,
  - widać numer N/30 (także na wydruku „(greg. 14/30)”), przerwy w serii i Msze współdzielone z innymi intencjami,
  - ofiarę za całą serię wpisuje się raz.
- **Wpłaty**: kwota przy intencji (tylko w programie, nigdy na wydruku), suma tygodnia, wyróżnianie intencji bez kwoty i podsumowania za tydzień, miesiąc i rok. Szybkie kwoty ustawia się w Ustawieniach.
- **Święta i wspomnienia dnia**: wpisywane ręcznie albo pobierane przyciskiem „Pobierz święta z GCatholic” (wymaga internetu; rodzaje obchodów do wyboru w ustawieniach).
- **Wydruk PDF** całego tygodnia: nagłówek z nazwą i adresem parafii, osobna numeracja intencji parafialnych i „poza parafią”. Tydzień zwykle mieści się na jednej stronie A4 (lekko zmniejszony tekst), a gdy intencji jest więcej, przechodzi na kolejną; dzień, który nie zmieściłby się nawet na całej stronie, dzieli się na dwie z dopiskiem „ciąg dalszy”.
- **Ukrywanie na wydruku**: checkbox „Ukryj na wydruku” w formularzu. Ukryta intencja zostaje w widoku tygodnia (wyszarzona, z oznaczeniem), a w podglądzie wydruku można ją dołączyć checkboxem „Pokaż ukryte intencje”.
- **Wydruk z miejscami do wpisania** (opcja „Miejsca do wpisania” w podglądzie wydruku), żeby uzupełnić tydzień długopisem:
  - Msze bez intencji parafialnej dostają wykropkowane linie (1–3 na Mszę),
  - opcjonalnie dodatkowa linia pod uzupełnionymi Mszami,
  - dla tygodnia bez wczytanych godzin można dodać godziny z porządku Mszy, więc da się wydrukować zupełnie pusty tydzień,
  - Msza z ukrytą intencją nigdy nie jest pokazywana jako pusta.
- **Ochrona przed utratą danych**: każda intencja jest zapisywana do bazy w chwili kliknięcia „Zapisz”. Tekst wpisywany w formularzu program na bieżąco zapisuje jako szkic. Po zaniku prądu albo zawieszeniu się programu przy następnym uruchomieniu pojawia się pasek „Niezapisana intencja – Przywróć / Odrzuć”. Przy starcie program sprawdza też, czy baza nie jest uszkodzona.
- **Wyczyść tydzień** (z potwierdzeniem).
- **Automatyczna kopia danych** (np. do Google Drive / OneDrive): w Ustawieniach wybierasz folder, a przycisk „Wykonaj kopię danych” na górnym pasku zapisuje tam plik `intencje-kopia.db` (nadpisywany). Pasek pokazuje, czy od ostatniej kopii coś się zmieniło. Przy zamykaniu programu kopia robi się sama, jeśli są zmiany; pytanie pojawia się tylko, gdy się nie uda. Folder i czas ostatniej kopii są w `kopia.json` obok bazy (import ich nie nadpisuje).
- **Eksport i import danych** (Ustawienia → Kopia danych): eksport zapisuje całą bazę (intencje, święta, ustawienia, porządki mszy) do jednego pliku `.db` przez okno „Zapisz jako”. Import pokazuje, co jest w pliku, i po potwierdzeniu zastępuje nim obecne dane; wcześniej zapisuje kopię `data-RRRR-MM-DD-GGMMSS.db.bak` w folderze danych (`%AppData%\Go Parafia`). Import takiego pliku `.bak` cofa zamianę.
- **Ustawienia** (ikona koła zębatego): nazwa i adres parafii, pobieranie świąt, porządki mszy, szybkie kwoty wpłat.
- Numer wersji programu widać na ekranie powitalnym.
- **Aktualizacje z GitHuba** (Ustawienia → Aktualizacje): „Sprawdź aktualizacje” pokazuje, czy jest nowsza wersja i co się w niej zmieniło. „Pobierz i zainstaluj” wykonuje kolejno:
  1. zapisuje kopię danych (`data-…-przed-aktualizacja.db.bak` w folderze danych),
  2. pobiera nowy program i sprawdza jego sumę kontrolną,
  3. podmienia plik `.exe`,
  4. uruchamia program ponownie.

  Przez cały czas widać stronę z etapami i paskiem postępu pobierania („Pobieram nową wersję… 12,3 MB z 20,3 MB”). Jeśli coś się nie uda, ta sama strona pokazuje błąd i przycisk „Spróbuj ponownie”.

  Jeśli Windows nie pozwoli uruchomić nowej wersji, zostaje poprzednia. Program sprawdza też aktualizacje sam przy starcie (gdy jest internet) i wtedy pokazuje informację na ekranie powitalnym.

## Uruchomienie u użytkownika (Windows)

1. Skopiuj `Intencje Mszalne.exe` w dowolne miejsce (np. na pulpit) i uruchom.
2. Windows 10/11 ma już potrzebny silnik okna (WebView2), więc nic nie trzeba doinstalowywać.
3. Program uruchamia się tylko raz. Ponowne kliknięcie pokazuje już otwarte okno.

Dane programu:

| Co | Gdzie |
|---|---|
| baza intencji, ustawień, porządków | `%AppData%\Go Parafia\data.db` |
| folder i czas automatycznej kopii | `%AppData%\Go Parafia\kopia.json` |
| szkic niezapisanej intencji | `%AppData%\Go Parafia\szkic-intencji.json` |
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

Wydajność na dużej bazie (30 lat intencji, ok. 80 tys.) sprawdzają benchmarki uruchamiane na żądanie:

```bash
go test ./internal/web -run '^$' -bench LargeParish
```

- Wersja deweloperska (`go run .`) to zwykły serwer HTTP na `127.0.0.1:8080`, a `.exe` to okno [Wails](https://wails.io) bez portu sieciowego. Wybór następuje przy kompilacji przez tagi (`server.go` vs `desktop.go` z `//go:build desktop`).
- Szablony, CSS, JS, czcionki PDF i domyślne porządki mszy są wkompilowane w program (`//go:embed`). Po ich zmianie trzeba uruchomić program od nowa.
- Numer wersji, ikona i opis pliku `.exe`: `build/windows/winres.json` (narzędzie `go-winres`, uruchamiane przez `make windows`).

### Struktura

```
main.go, routes.go          start programu, router
server.go / desktop.go      serwer HTTP (dev) albo okno Wails (.exe)
internal/intentions         intencje: model, walidacja, zapytania tygodnia
internal/masses             porządki mszy: domyślne (schedules.json), baza, sloty, Msze w zakresie dat (wyszukiwarka)
internal/gregorian          serie Mszy gregoriańskich
internal/calendar           opisy dni (święta, wspomnienia)
internal/gcatholic          pobieranie i odczyt kalendarza GCatholic (iCal)
internal/settings           ustawienia parafii
internal/database           otwarcie bazy, migracje, eksport, import z kopią .bak
internal/backup             automatyczna kopia do wybranego folderu, śledzenie zmian
internal/draft              szkic formularza intencji (odzyskiwanie po awarii)
internal/safefile           zapis plików z wymuszeniem zapisu na dysk
internal/update             sprawdzanie i instalacja nowych wersji z GitHub Releases
internal/web                strony HTML, formularze, wyszukiwarka, wydruk PDF
internal/api                JSON API (/api/intentions)
```

### Wydanie nowej wersji

1. Podbij wersję w `build/windows/winres.json` (pięć miejsc, np. `1.0.1` i `1.0.1.0`) i scal zmiany do `master`.
2. Oznacz wydanie tagiem z opisem zmian po polsku. Ten opis program pokaże w Ustawieniach.

   ```bash
   git tag -a v1.0.1 -m "Poprawki wydruku."
   git push origin v1.0.1
   ```
3. GitHub Actions (`.github/workflows/release.yml`) wykonuje kolejno:
   1. sprawdza, czy tag zgadza się z wersją w `winres.json`,
   2. uruchamia testy,
   3. buduje `.exe`,
   4. publikuje wydanie z plikami `Intencje-Mszalne.exe` i `SHA256SUMS.txt`.

   Z tego wydania korzysta przycisk „Pobierz i zainstaluj” w programie.

### CI

GitHub Actions przy każdym pushu i pull requeście: formatowanie, `go vet` (także wersji Windows), `go mod tidy`, testy z `-race`, `govulncheck`, `golangci-lint`.

## Licencja

Program jest udostępniony na licencji [MIT](LICENSE).

Czcionki Liberation Serif wbudowane w wydruk PDF mają własną licencję, SIL Open Font License 1.1 ([internal/web/fonts/LICENSE-Liberation.txt](internal/web/fonts/LICENSE-Liberation.txt)).

## Autor

Tomasz Kaliński, Rzezawa
