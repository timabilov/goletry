package languageutil

import (
	"math/rand"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var TitleCaser = cases.Title(language.Azerbaijani)
var LowerCaser = cases.Lower(language.Azerbaijani)
var Adjs []string = []string{
	"qəliz",
	"iti",
	"cəsur",
	"məyus",
	"itkin",
	"yumru",
	"nazik",
	"gizli",
	"vacib",
	"qızıl",
	"yaşıl",
	"qırmızı",
	"qara",
	"arıq",
	"əsəbi",
	"diqqətli",
	"varlı",
	"gözəl",
	"inadkar",
	"turş",
	"acı",
	"yarım",
	"şən",
	"tənbəl",
	"quru",
	"müasir",
	"sarı",
	"təzə",
	"hazır",
	"naməlum",
	"fantastik",
	"abırlı",
	"mücərrəd",
	"babat",
	"əcaib",
	"əcnəbi",
	"geniş",
	"nadir",
	"orijinal",

}

var Nouns []string = []string{
	"zarafat",
	"balış",
	"balıq",
	"balıq",
	"püstə",
	"misra",
	"divar",
	"limon",
	"böyrək",
	"mühit",
	"alma",
	"qıfıl",
	"həftə",
	"düymə",
	"külək",
	"fikir",
	"xalça",
	"kölgə",
	"lətifə",
	"zaman",
	"almaz",
	"dilək",
	"xəyal",
	"duyğu",
	"çanta",
	"qələm",
	"şikayət",
	"fasilə",
	"dünya",
	"səs",
	"vahimə",
	"yağış",
	"ağrı",
	"kədər",
	"yaddaş",
	"qaşıq",
	"teorem",
}

func RandomAdjective() string {

	pick := rand.Intn(len(Adjs))
	return Adjs[pick]
}

func RandomNounlike() string {

	pick := rand.Intn(len(Nouns))
	return Nouns[pick]
}
