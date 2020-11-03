package main

import (
	"database/sql"
	"flag"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	// register sqlite3 for database/sql
	_ "github.com/mattn/go-sqlite3"

	"hawx.me/code/indieauth"
	"hawx.me/code/indieauth/sessions"
	"hawx.me/code/serve"
)

func main() {
	var (
		url    = flag.String("url", "http://localhost:8080", "")
		secret = flag.String("secret", "h59CB0Jhr0V+w1PkuNSvaSpBrS10qZ8RUYim6w1XD44=", "")
		me     = flag.String("me", "", "")

		dbPath = flag.String("db", "db", "")

		port   = flag.String("port", "8080", "")
		socket = flag.String("socket", "", "")
	)

	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Println(err)
		return
	}
	defer db.Close()

	mux, err := newMux(*url, *me, *secret, db)
	if err != nil {
		log.Println(err)
		return
	}

	serve.Serve(*port, *socket, mux)
}

func newMux(url, me, secret string, db *sql.DB) (http.Handler, error) {
	mux := http.NewServeMux()

	auth, err := indieauth.Authentication(url, url+"/callback")
	if err != nil {
		return mux, err
	}

	session, err := sessions.New(me, secret, auth)
	if err != nil {
		return mux, err
	}

	tmpl, err := template.New("textbox").Parse(textboxTmpl)
	if err != nil {
		return mux, err
	}

	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS textbox (
      id NUMBER PRIMARY KEY,
      content TEXT,
      updated_at DATETIME
    );

    INSERT OR IGNORE INTO textbox(id, content, updated_at)
      VALUES (0, '', time('now'));`)
	if err != nil {
		return mux, err
	}

	mux.Handle("/", session.Choose(
		http.HandlerFunc(handleDisplay(db, tmpl)),
		http.HandlerFunc(handleSignIn)))
	mux.HandleFunc("/save", session.Shield(handleSave(db)))

	mux.HandleFunc("/sign-in", session.SignIn())
	mux.HandleFunc("/callback", session.Callback())
	mux.HandleFunc("/sign-out", session.SignOut())

	return mux, nil
}

func handleDisplay(db *sql.DB, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "", http.StatusMethodNotAllowed)
			return
		}

		row := db.QueryRowContext(
			r.Context(),
			"SELECT content, updated_at FROM textbox")

		var v struct {
			Content   string
			UpdatedAt time.Time
		}
		if err := row.Scan(&v.Content, &v.UpdatedAt); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tmpl.Execute(w, v); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func handleSignIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	io.WriteString(w, signInTmpl)
}

func handleSave(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "", http.StatusMethodNotAllowed)
			return
		}

		content := r.FormValue("textbox")
		updatedAt := time.Now()

		_, err := db.ExecContext(
			r.Context(),
			"UPDATE textbox SET content = ?, updated_at = ?",
			content, updatedAt)

		if err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

const textboxTmpl = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>textbox</title>
    <style>
       html, body { height: 100%; width: 100%; margin: 0; padding: 0; }
       form { height: 100%; display: flex; flex-direction: column; }
       textarea { display: block; flex: 1; resize: none; padding: 1rem 1.3rem; border: none; font: 1rem/1.5 monospace; }
       form div { display: flex; justify-content: space-between; margin: .5rem .7rem; }
       button { border: 1px solid; color: blue; background: none; border-radius: .2rem; padding: .2rem .5rem; }
       button:not(:disabled):hover { background: blue; color: white; border-color: black; }
       button:not(:disabled):active { box-shadow: 0 0 0 3px orange; }
       button:disabled { color: silver; }
       time { color: silver; font: .8rem monospace; }
    </style>
  </head>
  <body>
    <form action="/save" method="post">
      <textarea name="textbox">{{ .Content }}</textarea>
      <div>
        <button type="submit" disabled>Save</button>
        <time>{{ .UpdatedAt }}</time>
      </div>
    </form>
    <script>
      const form = document.querySelector('form');
      const button = document.querySelector('button');

      function save(event) {
        if ((event.ctrlKey || event.metaKey) && String.fromCharCode(event.which).toLowerCase() === 's') {
          event.preventDefault();
          form.submit();
        }
      }

      document.onkeydown = (event) => {
        button.disabled = false;
        document.onkeydown = save;
      };
    </script>
  </body>
</html>`

const signInTmpl = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>textbox</title>
  </head>
  <body>
    <div id="cover">
      <a href="/sign-in">Sign-in</a>
    </div>
  </body>
</html>`
