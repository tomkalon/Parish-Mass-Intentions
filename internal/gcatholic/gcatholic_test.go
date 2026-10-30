package gcatholic

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
)

func readSample(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile("testdata/sample.ics")
	if err != nil {
		t.Fatalf("reading sample: %v", err)
	}

	return string(data)
}

var sampleCelebrations = []Celebration{
	{Date: "2026-01-02", Rank: "W", Name: "Świętych Bazylego Wielkiego i Grzegorza z Nazjanzu, biskupów i doktorów Kościoła"},
	{Date: "2026-02-23", Rank: "w*", Name: "Św. Polikarpa, biskupa i męczennika"},
	{Date: "2026-11-01", Rank: "U", Name: "Wszystkich Świętych"},
	{Date: "2026-11-03", Rank: "", Name: "Wtorek XXXI tygodnia zwykłego"},
	{Date: "2026-11-03", Rank: "w", Name: "Św. Marcina de Porres, zakonnika"},
	{Date: "2026-11-04", Rank: "W", Name: "Św. Karola Boromeusza, biskupa"},
	{Date: "2026-11-08", Rank: "", Name: "XXXII Niedziela Zwykła"},
	{Date: "2026-11-09", Rank: "Ś", Name: "Rocznica poświęcenia Bazyliki Laterańskiej"},
}

func TestParse(t *testing.T) {
	sample := readSample(t)

	tests := []struct {
		name string
		data string
	}{
		{name: "LF line endings", data: sample},
		{name: "CRLF line endings like GCatholic", data: strings.ReplaceAll(sample, "\n", "\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.data))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if !slices.Equal(got, sampleCelebrations) {
				t.Errorf("Parse =\n%q\nwant\n%q", got, sampleCelebrations)
			}
		})
	}
}

func TestParseInvalidDate(t *testing.T) {
	data := "BEGIN:VEVENT\nDTSTART;VALUE=DATE:2026-11-04\nSUMMARY:⚪ [W] Św. Karola\nEND:VEVENT\n"

	_, err := Parse(strings.NewReader(data))
	if err == nil {
		t.Error("Parse error = nil, want an error for a malformed DTSTART")
	}
}

func TestCelebrationText(t *testing.T) {
	tests := []struct {
		celebration Celebration
		want        string
	}{
		{Celebration{Rank: "U", Name: "Wszystkich Świętych"}, "uroczystość Wszystkich Świętych"},
		{Celebration{Rank: "Ś", Name: "Rocznica poświęcenia Bazyliki Laterańskiej"}, "święto Rocznica poświęcenia Bazyliki Laterańskiej"},
		{Celebration{Rank: "W", Name: "Św. Karola Boromeusza, biskupa"}, "wspomnienie św. Karola Boromeusza, biskupa"},
		{Celebration{Rank: "w", Name: "Św. Marcina de Porres, zakonnika"}, "wspomnienie dowolne św. Marcina de Porres, zakonnika"},
		{Celebration{Rank: "w*", Name: "Św. Polikarpa, biskupa i męczennika"}, "wspomnienie dowolne św. Polikarpa, biskupa i męczennika"},
		{Celebration{Rank: "", Name: "XXXII Niedziela Zwykła"}, "XXXII Niedziela Zwykła"},
		{Celebration{Rank: "", Name: "Św. Szczepana"}, "Św. Szczepana"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.celebration.Text(); got != tt.want {
				t.Errorf("Text() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTexts(t *testing.T) {
	got := Texts(sampleCelebrations)

	want := map[string]string{
		"2026-01-02": "wspomnienie Świętych Bazylego Wielkiego i Grzegorza z Nazjanzu, biskupów i doktorów Kościoła",
		"2026-02-23": "wspomnienie dowolne św. Polikarpa, biskupa i męczennika",
		"2026-11-01": "uroczystość Wszystkich Świętych",
		"2026-11-03": "Wtorek XXXI tygodnia zwykłego; wspomnienie dowolne św. Marcina de Porres, zakonnika",
		"2026-11-04": "wspomnienie św. Karola Boromeusza, biskupa",
		"2026-11-08": "XXXII Niedziela Zwykła",
		"2026-11-09": "święto Rocznica poświęcenia Bazyliki Laterańskiej",
	}
	if !maps.Equal(got, want) {
		t.Errorf("Texts =\n%q\nwant\n%q", got, want)
	}
}

func TestFetch(t *testing.T) {
	sample := readSample(t)

	var gotPath, gotQuery, gotAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAgent = r.URL.Path, r.URL.RawQuery, r.UserAgent()
		_, _ = w.Write([]byte(sample))
	}))
	defer server.Close()

	got, err := Fetch(context.Background(), server.Client(), server.URL, 2026)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if gotPath != "/calendar/ics/2026-pl-PL.ics" || gotQuery != "v=3" {
		t.Errorf("requested %s?%s, want /calendar/ics/2026-pl-PL.ics?v=3", gotPath, gotQuery)
	}
	if gotAgent != "Go Parafia" {
		t.Errorf("User-Agent = %q, want %q", gotAgent, "Go Parafia")
	}
	if !slices.Equal(got, sampleCelebrations) {
		t.Errorf("Fetch returned %d celebrations, want the parsed sample", len(got))
	}
}

func TestFetchErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	_, err := Fetch(context.Background(), server.Client(), server.URL, 2026)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("Fetch error = %v, want one mentioning status 404", err)
	}
}

func TestFetchServerDown(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()

	_, err := Fetch(context.Background(), http.DefaultClient, url, 2026)
	if err == nil {
		t.Error("Fetch error = nil, want an error when the server is unreachable")
	}
}

func TestCelebrationKind(t *testing.T) {
	tests := []struct {
		rank string
		name string
		want Kind
	}{
		{rank: "U", name: "Wszystkich Świętych", want: Solemnity},
		{rank: "Ś", name: "Rocznica poświęcenia Bazyliki Laterańskiej", want: Feast},
		{rank: "W", name: "Św. Karola Boromeusza, biskupa", want: Memorial},
		{rank: "w", name: "Św. Marcina de Porres, zakonnika", want: OptionalMemorial},
		{rank: "w*", name: "Św. Polikarpa, biskupa i męczennika", want: OptionalMemorial},
		{name: "Wtorek XXXI tygodnia zwykłego", want: Weekday},
		{name: "Poniedziałek II Tygodnia Wielkanocnego", want: Weekday},
		{name: "Sobota przed Objawieniem Pańskim", want: Weekday},
		{name: "Czwartek po Popielcu", want: Weekday},
		{name: "Dzień adwentu (17 grudnia)", want: Weekday},
		{name: "XXXII Niedziela Zwykła", want: Other},
		{name: "Środa Popielcowa", want: Other},
		{name: "Wielki Czwartek: Wieczerzy Pańskiej", want: Other},
		{name: "Wspomnienie wszystkich wiernych zmarłych", want: Other},
		{name: "V dzień w oktawie Narodzenia Pańskiego", want: Other},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Celebration{Rank: tt.rank, Name: tt.name}).Kind(); got != tt.want {
				t.Errorf("Kind() = %d, want %d", got, tt.want)
			}
		})
	}
}
