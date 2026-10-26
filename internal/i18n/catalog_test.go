package i18n

import (
    "errors"
    "reflect"
    "strings"
    "testing"
)

func TestCatalogsHaveExactEnglishGermanThaiParity(t *testing.T) {
    supported:=Supported()
    if !reflect.DeepEqual(supported,[]Locale{English,German,Thai}) {
        t.Fatalf("unexpected initial language catalog: %v",supported)
    }
    all:=map[Locale]map[string]string{}
    for _,locale:=range supported {
        c,err:=Catalog(locale)
        if err!=nil{t.Fatal(err)}
        if len(c)<50{t.Fatalf("incomplete catalog %s: %d",locale,len(c))}
        all[locale]=c
    }
    if err:=ValidateCatalogs(all);err!=nil{t.Fatal(err)}
    all[German]["login.title"]=""
    if err:=ValidateCatalogs(all);!errors.Is(err,ErrInvalidCatalog) {
        t.Fatalf("empty German translation passed: %v",err)
    }
    all[German]["login.title"]="Ein Titel"
    delete(all[Thai],"overview.last_print")
    if err:=ValidateCatalogs(all);!errors.Is(err,ErrInvalidCatalog){
        t.Fatalf("missing Thai translation passed: %v",err)
    }
}

func TestCatalogPlaceholderDriftAndHTMLAreRejected(t *testing.T){
    all:=map[Locale]map[string]string{}
    for _,locale:=range Supported() {
        c,err:=Catalog(locale)
        if err!=nil{t.Fatal(err)}
        all[locale]=c
    }
    all[Thai]["overview.last_print"]="พิมพ์ล่าสุด: {wrong}"
    if err:=ValidateCatalogs(all);!errors.Is(err,ErrInvalidCatalog){
        t.Fatalf("changed placeholder accepted: %v",err)
    }
    all[Thai]["overview.last_print"]="พิมพ์ล่าสุด: {time}"
    all[German]["login.title"]="<strong>Hallo</strong>"
    if err:=ValidateCatalogs(all);!errors.Is(err,ErrInvalidCatalog){
        t.Fatalf("markup found in plain-text catalog: %v",err)
    }
}

func TestLocaleSelectionAndFallback(t *testing.T) {
    for _,c:=range []struct{input string;want Locale}{
        {"th-TH",Thai},{"TH_th",Thai},{"de-DE",German},{"de",German},
        {"en-US",English},{"fr-FR",English},{"",English},{"*",English},
        {" th@invalid ",English},{"en;admin",English},{"th-",English},
        {strings.Repeat("de",40),English},
    }{
        if got:=Normalize(c.input);got!=c.want {t.Errorf("%q got %s want %s",c.input,got,c.want)}
    }
    for _,c:=range []struct{input string;want Locale}{
        {"de-DE;q=0.4,th-TH;q=0.8,en-US;q=0.2",Thai},
        {"fr-FR;q=1,de-DE;q=0.4",German},
        {"de-DE;q=0,th-TH;q=0.6",Thai},
        {"en-US;q=0.6,de-DE;q=0.6",English},
        {"de-DE;q=banana,th-TH;q=0.8",Thai},
        {"*;q=0.9,es-ES;q=0.8",English},
        {"",English},{strings.Repeat("th,",500),English},
    }{
        if got:=Negotiate(c.input);got!=c.want {t.Errorf("header %q got %s want %s",c.input,got,c.want)}
    }
}

func TestMessagesArePlainTextAndNeverClaimAuthority(t *testing.T) {
    for _,lang:=range Supported(){
        msg,err:=Message(lang,"overview.last_print",map[string]string{"time":"2026-10-25T10:00:00Z"})
        if err!=nil||!strings.Contains(msg,"2026-10-25T10:00:00Z"){
            t.Fatalf("missing placeholder %s: %q %v",lang,msg,err)
        }
        if _,err:=Message(lang,"overview.last_print",nil);!errors.Is(err,ErrInvalidCatalog){
            t.Fatalf("silent unresolved locale interpolation %s: %v",lang,err)
        }
        if _,err:=Message(lang,"an_unsupported_authorization_or_status",nil);!errors.Is(err,ErrInvalidCatalog){
            t.Fatalf("unknown message key incorrectly accepted %s: %v",lang,err)
        }
    }
    injected:="untrusted text from external input"
    msg,err:=Message(German,"diagnostics.next_action",map[string]string{"action":injected})
    if err!=nil||!strings.Contains(msg,injected){t.Fatalf("replacement altered or executed: %q %v",msg,err)}
    // It is a string for textContent. No HTML parser or remote request exists
    // here; callers must independently enforce text-node insertion.
}

func TestCatalogCopiesCannotAlterGlobalOrClaimSupportedLocale(t *testing.T){
    first,err:=Catalog(Thai)
    if err!=nil{t.Fatal(err)}
    original:=first["login.title"]
    first["login.title"]="a changed client-owned copy"
    second,err:=Catalog(Thai)
    if err!=nil||second["login.title"]!=original{t.Fatal("catalog mutation changed global text")}
    unknown,err:=Catalog(Locale("fr"))
    if err!=nil{t.Fatal(err)}
    english,err:=Catalog(English)
    if err!=nil||!reflect.DeepEqual(unknown,english){t.Fatal("unsupported locale did not fall back to English")}
}
