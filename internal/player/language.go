package player

import "strings"

// languageNames maps ISO 639 codes to English names.
//
// Both the two-letter (639-1) and three-letter (639-2/B and /T) forms appear
// because containers are inconsistent: Matroska generally stores 639-2/B, MP4
// often stores 639-2/T, and files remuxed by consumer tools frequently carry the
// two-letter form. Mapping all three is what turns a subtitle menu reading
// "eng / gre / spa" into one reading "English / Greek / Spanish".
var languageNames = map[string]string{
	"ab": "Abkhazian", "aa": "Afar", "af": "Afrikaans", "ak": "Akan",
	"sq": "Albanian", "alb": "Albanian", "sqi": "Albanian",
	"am": "Amharic", "amh": "Amharic",
	"ar": "Arabic", "ara": "Arabic",
	"hy": "Armenian", "arm": "Armenian", "hye": "Armenian",
	"as": "Assamese", "az": "Azerbaijani", "aze": "Azerbaijani",
	"eu": "Basque", "baq": "Basque", "eus": "Basque",
	"be": "Belarusian", "bel": "Belarusian",
	"bn": "Bengali", "ben": "Bengali",
	"bs": "Bosnian", "bos": "Bosnian",
	"br": "Breton",
	"bg": "Bulgarian", "bul": "Bulgarian",
	"my": "Burmese", "bur": "Burmese", "mya": "Burmese",
	"ca": "Catalan", "cat": "Catalan",
	"km": "Khmer", "khm": "Khmer",
	"zh": "Chinese", "chi": "Chinese", "zho": "Chinese",
	"cmn": "Mandarin", "yue": "Cantonese",
	"hr": "Croatian", "hrv": "Croatian",
	"cs": "Czech", "cze": "Czech", "ces": "Czech",
	"da": "Danish", "dan": "Danish",
	"nl": "Dutch", "dut": "Dutch", "nld": "Dutch",
	"en": "English", "eng": "English",
	"eo": "Esperanto", "epo": "Esperanto",
	"et": "Estonian", "est": "Estonian",
	"fo": "Faroese",
	"fi": "Finnish", "fin": "Finnish",
	"fr": "French", "fre": "French", "fra": "French",
	"gl": "Galician", "glg": "Galician",
	"ka": "Georgian", "geo": "Georgian", "kat": "Georgian",
	"de": "German", "ger": "German", "deu": "German",
	"el": "Greek", "gre": "Greek", "ell": "Greek",
	"gu": "Gujarati", "guj": "Gujarati",
	"ht": "Haitian Creole",
	"ha": "Hausa",
	"he": "Hebrew", "heb": "Hebrew", "iw": "Hebrew",
	"hi": "Hindi", "hin": "Hindi",
	"hu": "Hungarian", "hun": "Hungarian",
	"is": "Icelandic", "ice": "Icelandic", "isl": "Icelandic",
	"ig": "Igbo",
	"id": "Indonesian", "ind": "Indonesian", "in": "Indonesian",
	"ga": "Irish", "gle": "Irish",
	"it": "Italian", "ita": "Italian",
	"ja": "Japanese", "jpn": "Japanese",
	"jv": "Javanese",
	"kn": "Kannada", "kan": "Kannada",
	"kk": "Kazakh", "kaz": "Kazakh",
	"ko": "Korean", "kor": "Korean",
	"ku": "Kurdish", "kur": "Kurdish",
	"ky": "Kyrgyz",
	"lo": "Lao", "lao": "Lao",
	"la": "Latin", "lat": "Latin",
	"lv": "Latvian", "lav": "Latvian",
	"lt": "Lithuanian", "lit": "Lithuanian",
	"lb": "Luxembourgish",
	"mk": "Macedonian", "mac": "Macedonian", "mkd": "Macedonian",
	"mg": "Malagasy",
	"ms": "Malay", "may": "Malay", "msa": "Malay",
	"ml": "Malayalam", "mal": "Malayalam",
	"mt": "Maltese", "mlt": "Maltese",
	"mi": "Maori", "mr": "Marathi", "mar": "Marathi",
	"mn": "Mongolian", "mon": "Mongolian",
	"ne": "Nepali", "nep": "Nepali",
	"no": "Norwegian", "nor": "Norwegian",
	"nb": "Norwegian Bokmal", "nob": "Norwegian Bokmal",
	"nn": "Norwegian Nynorsk", "nno": "Norwegian Nynorsk",
	"oc": "Occitan",
	"or": "Odia", "om": "Oromo",
	"ps": "Pashto", "pus": "Pashto",
	"fa": "Persian", "per": "Persian", "fas": "Persian",
	"pl": "Polish", "pol": "Polish",
	"pt": "Portuguese", "por": "Portuguese",
	"pa": "Punjabi", "pan": "Punjabi",
	"qu": "Quechua",
	"ro": "Romanian", "rum": "Romanian", "ron": "Romanian",
	"ru": "Russian", "rus": "Russian",
	"sa": "Sanskrit",
	"gd": "Scottish Gaelic",
	"sr": "Serbian", "srp": "Serbian",
	"sn": "Shona",
	"sd": "Sindhi",
	"si": "Sinhala", "sin": "Sinhala",
	"sk": "Slovak", "slo": "Slovak", "slk": "Slovak",
	"sl": "Slovenian", "slv": "Slovenian",
	"so": "Somali", "som": "Somali",
	"st": "Southern Sotho",
	"es": "Spanish", "spa": "Spanish",
	"su": "Sundanese",
	"sw": "Swahili", "swa": "Swahili",
	"sv": "Swedish", "swe": "Swedish",
	"tl": "Tagalog", "tgl": "Tagalog", "fil": "Filipino",
	"tg": "Tajik",
	"ta": "Tamil", "tam": "Tamil",
	"tt": "Tatar",
	"te": "Telugu", "tel": "Telugu",
	"th": "Thai", "tha": "Thai",
	"bo": "Tibetan", "tib": "Tibetan", "bod": "Tibetan",
	"ti": "Tigrinya",
	"tr": "Turkish", "tur": "Turkish",
	"tk": "Turkmen",
	"uk": "Ukrainian", "ukr": "Ukrainian",
	"ur": "Urdu", "urd": "Urdu",
	"ug": "Uyghur",
	"uz": "Uzbek", "uzb": "Uzbek",
	"vi": "Vietnamese", "vie": "Vietnamese",
	"cy": "Welsh", "wel": "Welsh", "cym": "Welsh",
	"fy": "Western Frisian",
	"xh": "Xhosa",
	"yi": "Yiddish",
	"yo": "Yoruba",
	"zu": "Zulu",
	"und": "Unknown", "mul": "Multiple languages", "zxx": "No linguistic content",
}

// LanguageName converts a language code to a display name.
//
// Codes may carry a region or script subtag (pt-BR, zh-Hans), so only the
// primary subtag is looked up and the remainder is appended in parentheses.
// A code with no entry is returned upper-cased, which reads as a deliberate
// label rather than as a bug.
func LanguageName(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}

	normalised := strings.ToLower(strings.ReplaceAll(code, "_", "-"))
	primary, rest, hasRest := strings.Cut(normalised, "-")

	name, ok := languageNames[primary]
	if !ok {
		return strings.ToUpper(code)
	}
	if hasRest && rest != "" {
		return name + " (" + strings.ToUpper(rest) + ")"
	}
	return name
}
