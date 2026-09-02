package i18nLoc

import (
	"encoding/json"
	"log"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"io/ioutil"
	"os"
	"path"
)

var bundle *i18n.Bundle
var loc *i18n.Localizer

// LocLan loads the local language files (English only)
func LocLan(locLanguage string) {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	_, err := os.Stat("i18n")
	if err != nil {
		err := os.Mkdir("i18n", 0666)
		dropErr(err)
	}
	rd, err := ioutil.ReadDir("i18n")
	dropErr(err)
	for _, fi := range rd {
		if !fi.IsDir() && path.Ext(fi.Name()) == ".json" {
			_, err := bundle.LoadMessageFile("i18n/" + fi.Name())
			dropErr(err)
		}
	}
	loc = i18n.NewLocalizer(bundle, locLanguage)
}

// LocText translates a json key to the target language
func LocText(MessageIDs ...string) string {
	res := ""
	for _, MessageID := range MessageIDs {
		res += loc.MustLocalize(&i18n.LocalizeConfig{MessageID: MessageID})
	}
	return res
}

func dropErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
