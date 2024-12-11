package main

import (
	"database/sql"
	"flag"
	"io"
	"log"
	"net/http"
	"time"

	// register sqlite3 for database/sql
	_ "github.com/mattn/go-sqlite3"

	"hawx.me/code/indieauth/v2"
	"hawx.me/code/lmth"
	"hawx.me/code/lmth/elements"
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

	if err := run(*port, *socket, *url, *me, *secret, *dbPath); err != nil {
		log.Fatal(err)
	}
}

func run(port, socket, url, me, secret, dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	session, err := indieauth.NewSessions(secret, &indieauth.Config{
		ClientID:    url,
		RedirectURL: url + "/callback",
	})
	if err != nil {
		return err
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
		return err
	}

	mux := http.NewServeMux()

	choose := func(a, b http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if response, ok := session.SignedIn(r); ok && response.Me == me {
				a.ServeHTTP(w, r)
			} else {
				b.ServeHTTP(w, r)
			}
		}
	}

	signedIn := func(a http.Handler) http.HandlerFunc {
		return choose(a, http.NotFoundHandler())
	}

	mux.Handle("/", choose(handleDisplay(db), handleSignIn()))
	mux.Handle("/save", signedIn(handleSave(db)))

	mux.HandleFunc("/sign-in", func(w http.ResponseWriter, r *http.Request) {
		if err := session.RedirectToSignIn(w, r, me); err != nil {
			log.Println(err)
		}
	})
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if err := session.Verify(w, r); err != nil {
			log.Println(err)
		}
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("/sign-out", func(w http.ResponseWriter, r *http.Request) {
		if err := session.SignOut(w, r); err != nil {
			log.Println(err)
		}
		http.Redirect(w, r, "/", http.StatusFound)
	})

	serve.Serve(port, socket, mux)
	return nil
}

func handleDisplay(db *sql.DB) http.HandlerFunc {
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

		if err := renderTextbox(w, v.Content, v.UpdatedAt); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func handleSignIn() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "", http.StatusMethodNotAllowed)
			return
		}

		renderSignIn(w)
	}
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

func renderTextbox(w io.Writer, content string, updatedAt time.Time) error {
	w.Write([]byte("<!DOCTYPE html>"))

	doc := elements.Html(lmth.Attr{"lang": "en"},
		elements.Head(lmth.Attr{},
			elements.Meta(lmth.Attr{"charset": "utf-8"}),
			elements.Meta(lmth.Attr{"name": "viewport", "content": "width=device-width, initial-scale=1.0"}),
			elements.Title(lmth.Attr{}, lmth.Text("textbox")),
			elements.Style(lmth.Attr{}, lmth.Text(`
				html, body { height: 100%; width: 100%; margin: 0; padding: 0; }
				form { height: 100%; display: flex; flex-direction: column; }
				textarea { display: block; flex: 1; resize: none; padding: 1rem 1.3rem; border: none; font: 1rem/1.5 monospace; }
				form div { display: flex; justify-content: space-between; margin: .5rem .7rem; }
				button { border: 1px solid; color: blue; background: none; border-radius: .2rem; padding: .2rem .5rem; }
				button:not(:disabled):hover { background: blue; color: white; border-color: black; }
				button:not(:disabled):active { box-shadow: 0 0 0 3px orange; }
				button:disabled { color: silver; }
				time { color: silver; font: .8rem monospace; }
			`)),
		),
		elements.Body(lmth.Attr{},
			elements.Form(lmth.Attr{"action": "/save", "method": "post"},
				elements.Textarea(lmth.Attr{"autofocus": "autofocus", "name": "textbox"}, lmth.Text(content)),
				elements.Div(lmth.Attr{},
					elements.Button(lmth.Attr{"type": "submit", "disabled": "disabled"}, lmth.Text("Save")),
					elements.Time(lmth.Attr{}, lmth.Text(updatedAt.Format(time.RFC3339))),
				),
			),
			elements.Script(lmth.Attr{}, lmth.Text(`
				const form = document.querySelector('form');
				const textarea = document.querySelector('textarea');
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

				const len = textarea.value.length;
				textarea.focus();
				textarea.setSelectionRange(len, len);
			`)),
		),
	)

	_, err := doc.WriteTo(w)
	return err
}

func renderSignIn(w io.Writer) error {
	w.Write([]byte("<!DOCTYPE html>"))

	doc := elements.Html(lmth.Attr{"lang": "en"},
		elements.Head(lmth.Attr{},
			elements.Meta(lmth.Attr{"charset": "utf-8"}),
			elements.Meta(lmth.Attr{"name": "viewport", "content": "width=device-width, initial-scale=1.0"}),
			elements.Title(lmth.Attr{}, lmth.Text("textbox")),
			elements.Style(lmth.Attr{}, lmth.Text(`
				#cover { top: 0; left: 0; z-index: 1000; position: absolute; height: 100%; width: 100%; background: rgba(0, 255, 255, .7); }
				#cover a { position: relative; display: block; left: 50%; top: 50%; text-align: center; width: 100px; margin-left: -50px; height: 50px; line-height: 50px; margin-top: -25px; font-size: 16px; font-weight: bold; border: 1px solid; }
			`)),
		),
		elements.Body(lmth.Attr{},
			elements.Div(lmth.Attr{"id": "cover"},
				elements.A(lmth.Attr{"href": "/sign-in"}, lmth.Text("Sign-in")),
			),
		),
	)

	_, err := doc.WriteTo(w)
	return err
}
