module github.com/gizzahub/gzh-cli-git

go 1.24.0

require (
	github.com/fsnotify/fsnotify v1.9.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/sync v0.18.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/gizzahub/gzh-cli-core => ../gzh-cli-core

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	golang.org/x/sys v0.13.0 // indirect
)
