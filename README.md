# Config(v2)
Nginx configuration style parser with golang

# Install
```
go get github.com/recoye/config
```

# Document
1) Create a new file(example.conf)

```
#daemon yes;
daemon no;
log_file run/log/file.log;
server {
    host 127.0.0.1;
    port 80;
}
```

2) Define configuration's struct

```
type ServConf struct {
    Host string
    Port int
}
type Environ struct {
    Daemon bool
    Log_file string
    Server ServConf
}
```

3) map file to struct

```
conf := config.New("example.conf")
env = &Environ{}
err = conf.Unmarshal(env)
```

# Example
main.go

```
package main

import (
    "github.com/recoye/config"
    "log"
)

type ServConf struct {
    Host string
    Port int
}
type Environ struct {
    Daemon bool
    Log_file string
    Server ServConf
}

func main(){
    conf := config.New("example.conf")
    env := &Environ{}
    err := conf.Unmarshal(env)
    if err == nil {
        log.Println(env)
    }else{
        log.Println(err)
    }
}

```

example.conf

```
#daemon yes;
daemon no;
log_file run/log/file.log;
server {
    host 127.0.0.1;
    port 80;
}
```

Result:

```
2017/07/19 18:48:25 &{false run/log/file.log {127.0.0.1 80}}
```
