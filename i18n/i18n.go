package i18nLoc

import (
	_ "embed"
	"encoding/json"
	"log"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"io/ioutil"
	"path"
)

var bundle *i18n.Bundle
var loc *i18n.Localizer

// defaultEnglish is embedded in the binary so it runs standalone with no
// sidecar files.
//
//go:embed active.en.json
var defaultEnglish []byte

// LocLan loads the local language files (English only).
// Embedded defaults always load; extra *.json files in ./i18n (if present)
// load on top as overrides.
func LocLan(locLanguage string) {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	if _, err := bundle.ParseMessageFileBytes(defaultEnglish, "active.en.json"); err != nil {
		dropErr(err)
	}
	if rd, err := ioutil.ReadDir("i18n"); err == nil {
		for _, fi := range rd {
			if !fi.IsDir() && path.Ext(fi.Name()) == ".json" {
				_, err := bundle.LoadMessageFile("i18n/" + fi.Name())
				dropErr(err)
			}
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
