package modfs_test

import (
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/dolmen-go/modfs"
	"github.com/dolmen-go/modfs/httpfs"
)

// /cached-only is faster as ses only the cached versions.
const goProxyURL = "https://proxy.golang.org/cached-only/"

func Example_goproxy() {
	hfs, err := httpfs.NewHTTPFS(http.DefaultClient, goProxyURL)
	if err != nil {
		log.Fatal(err)
	}

	goproxy := modfs.New(hfs)

	mod, err := goproxy.OpenModule("golang.org/x/tools")
	if err != nil {
		log.Fatal(err)
	}
	if strings.HasPrefix(mod.Latest.Version, "v") {
		fmt.Println("version OK")
	} else {
		fmt.Println(mod.Latest.Version)
	}

	// Output:
	// version OK
}

func ExampleVersion_goproxy() {
	hfs, err := httpfs.NewHTTPFS(http.DefaultClient, goProxyURL)
	if err != nil {
		log.Fatal(err)
	}

	goproxy := modfs.New(hfs)

	mod, err := goproxy.OpenModule("golang.org/x/tools")
	if err != nil {
		log.Fatal(err)
	}
	if strings.HasPrefix(mod.Latest.Version, "v") {
		fmt.Println("version OK")
	} else {
		fmt.Println(mod.Latest.Version)
	}

	v, err := mod.Version("v0.30.0")
	if err != nil {
		log.Fatal(err)
	}
	vfs, err := v.OpenFS()
	if err != nil {
		log.Fatal(err)
	}
	defer vfs.Close()
	gomod, err := fs.ReadFile(vfs, "go.mod")
	os.Stdout.Write(bytes.ReplaceAll(gomod, []byte{'\t'}, []byte("        ")))
	// Output:
	// version OK
	// module golang.org/x/tools
	//
	// go 1.22.0 // => default GODEBUG has gotypesalias=0
	//
	// require (
	//         github.com/google/go-cmp v0.6.0
	//         github.com/yuin/goldmark v1.4.13
	//         golang.org/x/mod v0.23.0
	//         golang.org/x/net v0.35.0
	//         golang.org/x/sync v0.11.0
	//         golang.org/x/telemetry v0.0.0-20240521205824-bda55230c457
	// )
	//
	// require golang.org/x/sys v0.30.0 // indirect
}
