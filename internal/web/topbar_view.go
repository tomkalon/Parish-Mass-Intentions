package web

import "go-parafia/internal/settings"

type topbarView struct {
	ParishName string
	Address    string
}

func newTopbarView(setting settings.Setting) topbarView {
	return topbarView{ParishName: setting.ParishName, Address: setting.Address}
}
