package web

type startView struct {
	ParishName string
	Address    string
	WeekURL    string
	Draft      *draftNotice
	Warning    string
	Version    string
}
