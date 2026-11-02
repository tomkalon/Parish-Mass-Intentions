package web

import (
	"time"

	"go-parafia/internal/backup"
	"go-parafia/internal/intentions"
	"go-parafia/internal/settings"
)

type topbarView struct {
	ParishName string
	Address    string

	BackupLabel   string
	BackupPending bool
}

func newTopbarView(setting settings.Setting, state backup.State, now time.Time) topbarView {
	return topbarView{
		ParishName: setting.ParishName,
		Address:    setting.Address,

		BackupLabel:   backupLabel(state, now),
		BackupPending: state.Pending(),
	}
}

func backupLabel(state backup.State, now time.Time) string {
	switch {
	case state.Dir == "":
		return "folder kopii nie jest wybrany"
	case state.LastBackup.IsZero():
		return "brak kopii"
	case state.Pending():
		return "zmiany bez kopii"
	default:
		return "kopia: " + backupTime(state.LastBackup, now)
	}
}

func backupTime(at, now time.Time) string {
	at = at.In(intentions.TimeZone)
	now = now.In(intentions.TimeZone)

	switch {
	case at.Format(intentions.DateLayout) == now.Format(intentions.DateLayout):
		return "dziś, " + clockLabel(at)
	case at.Year() == now.Year():
		return at.Format("2.1") + ", " + clockLabel(at)
	default:
		return at.Format("2.1.2006") + ", " + clockLabel(at)
	}
}
