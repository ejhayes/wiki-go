# Embedding Wiki-Go

The `wikigo` package runs the wiki inside another Go program, so one binary can serve the wiki and the program's own routes.

```go
import "wiki-go/wikigo"

wiki, err := wikigo.New(wikigo.Options{
    ConfigFile: "/data/wiki/config.yaml",
    OwnerUser:  "owner", // optional, see below
})
if err != nil {
    log.Fatal(err)
}

mux := http.NewServeMux()
mux.HandleFunc("/v1/ping", ping) // the program's own routes come first
mux.Handle("/", wiki)            // the wiki serves everything else
```

`New` runs the same start-up steps as the `wiki-go` binary. It writes a default `config.yaml` when none exists, creates the data folders under the config's `root_dir`, and returns an `http.Handler`. It does not register anything on `http.DefaultServeMux` and does not listen on a port. The `server.host` and `server.port` settings are ignored.

## Options

| Option | Meaning |
|--------|---------|
| `ConfigFile` | Path to the wiki's `config.yaml`. Required. |
| `OwnerUser` | A user from the config's `users` list. When set, every request is signed in as that user, so the wiki shows no login screen. Use it only when one trusted person can reach the wiki, for example on a loopback address. A request that carries another valid session keeps it. Because owner mode signs in without a cookie, it also answers only when the `Host` header names a loopback address, and it refuses a `POST`, `PUT`, `PATCH` or `DELETE` that a browser marks as coming from another site (an `Origin` that differs from the host, or `Sec-Fetch-Site` other than `same-origin` or `none`). A program that sends neither header is not affected. |
| `OnChange` | A `func(wikigo.Change) error` that the wiki calls after it writes a page change under `documents/` and before it answers. `Change.Action` is `create`, `edit`, `delete` or `move`, `Change.Path` is the page path such as `personal/cooking/bread`, and `Change.OldPath` is the path before a move. Save, create, delete, move, version restore, the links page editor and import report their pages. An upload, rename or delete of a page's file is an `edit` of that page. When the function returns an error, the wiki answers `422` with `{"success": false, "message": <error text>}`, writes nothing else, and the editor shows the message. An import runs after its request is answered, so a refused imported page becomes an import error. Changes to the home page are not reported. |

## Limits

- **One wiki per process.** The wiki keeps its state in package-level variables, so call `New` once.
- **Pages are files.** The wiki reads `documents/<path>/document.md` under `root_dir` on every request. A program can write or change a page file while the wiki runs, and the next request shows it. Changes made this way have no version history. Edits made through the wiki do.
- **Comments are files.** Each comment is `comments/<path>/<timestamp>_<user>.md` under `root_dir`.
- **Importing.** The package lives at the module root as `wiki-go/wikigo`. The module path is `wiki-go`, so a program that depends on a fork names it with a `replace` directive, for example `replace wiki-go => github.com/<owner>/wiki-go <version>`.
