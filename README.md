# modfs - client of the GOPROXY protocol

`modfs` is a Go library that allows to access the source code of [Go modules](https://go.dev/ref/mod)
available via a [GOPROXY](https://go.dev/ref/mod#goproxy-protocol) such as https://proxy.golang.org.

See examples in the [package documentation](https://pkg.go.dev/github.com/dolmen-go/modfs).

The [GOPROXY protocol](https://go.dev/ref/mod#goproxy-protocol).

## Features

From a Go module path:

* access the latest version available on the proxy
* access the list of versions available
* browse the files of the module via an [io/fs.FS](https://pkg.go.dev/io/fs#FS).

## License

```
Copyright 2025 Olivier Mengué

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```