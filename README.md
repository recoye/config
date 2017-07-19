# Config
Nginx configuration style parser with golang

# Document
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

Result:
```
2017/07/19 18:48:25 &{false run/log/file.log {127.0.0.1 80}}
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
