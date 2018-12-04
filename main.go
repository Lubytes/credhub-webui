package main

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"bytes"
	"encoding/json"
	"time"
  "log"

  "github.com/gorilla/sessions"
  "github.com/gorilla/mux"
  "github.com/dgrijalva/jwt-go"
)

type ClientStruct struct {
	ClientID     string
	ClientSecret string
}

type AuthResponse struct {
	AccessToken     string `json:"access_token"`
	Expiry int `json:"expires_in"`
	TokenType string `json:"token_type"`
	Scope string `json:"scope"`
	Error string `json:"error"`
	ErrorDesc string `json:"error_description"`
}

type AuthServerResponse struct {
	AuthServer struct {
		URL string `json:"url"`
	} `json:"auth-server"`
	App struct {
		Name string `json:"name"`
	} `json:"app"`
}

var (
	// key must be 16, 24 or 32 bytes long (AES-128, AES-192 or AES-256)
  keyval = os.Getenv("COOKIE_KEY")
	key         = []byte(keyval)
	store       = sessions.NewCookieStore(key)
	credhubServer = os.Getenv("CREDHUB_SERVER")
	uiSslCert = os.Getenv("UI_SSL_CERT")
	uiSslKey = os.Getenv("UI_SSL_KEY")
	cookieName = os.Getenv("COOKIE_NAME")
)

type CredentialsData struct {
	Credentials []struct {
		VersionCreatedAt time.Time `json:"version_created_at"`
		Name             string    `json:"name"`
	} `json:"credentials"`
}

type CredentialPageData struct {
	PageTitle string
	Credentials     []CredentialsData
}

func ListCredentials(w http.ResponseWriter, r *http.Request) {
  //set the access token from session
  session := GetSession(w, r)
  accessToken, _ := session.Values["access_token"].(string)

  //api call to make
  apiQuery := "/api/v1/data?name-like="
  //if we get a search query, add it to the api_query
  param1, ok := r.URL.Query()["search"]
  if ok {
    apiQuery = apiQuery+param1[0]
  }
  // call the credhub api to get all credentials
  // set up netClient for use later
  var netClient = &http.Client{
    Timeout: time.Second * 10,
  }
  http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //ignore cert for now FIX: add credhub and uaa certificate as environment variables on startup
	req, _ := http.NewRequest("GET", credhubServer+apiQuery, bytes.NewBuffer([]byte("")))
	req.Header.Add("authorization", "bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, reqErr := netClient.Do(req)
  if reqErr != nil {
    fmt.Println(reqErr)
  	http.Error(w, "Error", http.StatusBadRequest)
  	return
  }
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	credRespBytes := []byte(body)
	credResp := CredentialsData{}
  if credServErr := json.Unmarshal([]byte(credRespBytes), &credResp); credServErr != nil {
    fmt.Println(credServErr)
  }
  data := CredentialPageData{
		PageTitle: "List Credentials",
		Credentials: []CredentialsData{
      credResp,
    },
	}
  // use template
  tmpl := template.Must(template.ParseFiles("templates/credentials.html", "templates/base.html"))
	tmpl.ExecuteTemplate(w, "base", data)
}

func ReturnBlank(w http.ResponseWriter) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  fmt.Fprint(w, "")
}

func RedirectHome(w http.ResponseWriter) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  fmt.Fprint(w, "<meta http-equiv=\"refresh\" content=\"0;URL='/'\" />")
}

func RedirectLogin(w http.ResponseWriter) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  fmt.Fprint(w, "<meta http-equiv=\"refresh\" content=\"0;URL='/login'\" />")
}

func FaviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "favicon.ico")
}

func GetSession(w http.ResponseWriter, r *http.Request) (*sessions.Session) {
  session, err := store.Get(r, cookieName)
	if err != nil {
    fmt.Printf("session error")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
  return session
}

func ValidateToken(next http.HandlerFunc) http.HandlerFunc {
  return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
    session := GetSession(w, req)
    accessToken, setbool := session.Values["access_token"].(string)
    if setbool == true && accessToken == "" {
      RedirectLogin(w)
      //return
    } else {
      var p jwt.Parser
      token, _, _ := p.ParseUnverified(accessToken, &jwt.StandardClaims{})
      if err := token.Claims.Valid(); err != nil {
        //invalid
        RedirectLogin(w)
        //return
      } else {
        //valid
        next(w, req)
        //return
      }
    }
    //RedirectLogin(w)
    return
  })
}

func main() {
  if len(os.Getenv("COOKIE_KEY")) == 0 {
    log.Fatalln("COOKIE_KEY env var not set")
  }
  if len(os.Getenv("CREDHUB_SERVER")) == 0 {
    log.Fatalln("CREDHUB_SERVER env var not set")
  }
  if len(os.Getenv("COOKIE_NAME")) == 0 {
    log.Fatalln("COOKIE_NAME env var not set")
  }
  if len(os.Getenv("COOKIE_KEY")) == 0 {
    log.Fatalln("COOKIE_KEY env var not set")
  }
  if len(os.Getenv("UI_SSL_CERT")) == 0 {
    log.Fatalln("UI_SSL_CERT env var not set")
  }
  if len(os.Getenv("UI_SSL_KEY")) == 0 {
    log.Fatalln("UI_SSL_KEY env var not set")
  }

  log.SetFlags(log.Ldate | log.Ltime)
  r := mux.NewRouter()
	r.HandleFunc("/login", Login)
	r.HandleFunc("/logout", Logout)
	r.HandleFunc("/get", ValidateToken(GetCredentials))
	r.HandleFunc("/delete", ValidateToken(DeleteCredentials))
	r.HandleFunc("/generate/{credtype}", ValidateToken(GenerateCredentials))
  r.HandleFunc("/favicon.ico", FaviconHandler)
	r.HandleFunc("/", ValidateToken(ListCredentials))

  err := http.ListenAndServeTLS(":8443", uiSslCert, uiSslKey, LogRequest(r))
	if err != nil {
		fmt.Println(err)
	}
}

func LogRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}
