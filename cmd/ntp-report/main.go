package main

import (
	"log"
	"net/http"
	"os"

	"github.com/AndrewLester/ntp/internal/templates"
)

func main() {
	port := os.Getenv("REPORT_PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
		}
		// time, err := ntp.Time("0.pool.ntp.org")
		// if err != nil {
		// 	fmt.Fprintf(os.Stderr, "Error querying time %v", err)
		// 	templates.TemplateExecutor.ExecuteTemplate(w, "error.html.tmpl", data)
		// 	return
		// }
		// fmt.Printf("CF time: %v\n", time)

		// fmt.Printf("Our time: %v\n", time.Local())

		templates.TemplateExecutor.ExecuteTemplate(w, "index.html.tmpl", data)

	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
