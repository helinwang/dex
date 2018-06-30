## Build

### Build with Docker

```
$ git clone git@github.com:helinwang/dex.git
$ cd dex
$ docker run -v `pwd`:/root/env/gopath/src/github.com/helinwang/dex -it dex:build bash
$ cd /root/env/gopath/src/github.com/helinwang/dex
$ glide install
$ go test ./pkg/...
$ go build ./cmd/node/
```

### Build without Docker

- [Install the latest version of Go](https://golang.org/doc/install#install)

- Install [Barreto-Naehrig curves](https://github.com/dfinity/bn)

  - Ubuntu or OSX can use the latest prebuilt libraries in the readme
    page.
  
  - Install the include files and built libraries into `/usr/include`
    and `/usr/lib` respectively (or anywhere else the Go build
    toolchain can find).

  - Install dependencies `apt install llvm g++ libgmp-dev libssl-dev`,
    they are required by cgo when linking the libraries.

  - Test the installation by:
    ```
    $ go get github.com/dfinity/go-dfinity-crypto
    $ cd $GOPATH/src/github.com/dfinity/go-dfinity-crypto/bls
    $ go test
    ```

- Install package manager [Glide](https://glide.sh/)

- Download source and build
  ```
  $ go get github.com/helinwang/dex
  $ glide install
  $ go test ./pkg/...
  $ go build ./cmd/node/
  ```

## License

GPLv3
