Bond Socket
===
bond socket provider a server/client channel like ssh tunneling, it can forward port between multi node.

## Features
* auto reconnect and keepalive betwenn client to server.
* control forward by dynamic uri like `node1->node2->tcp://xxxx:xx`
* port forward between multi node, it can be `normal client -> bsck server -> normal server` or `normal client -> bsck server -> bsck slaver -> normal server` or `normal client -> bsck client -> bsck server -> bsck slaver -> normal server` or ...
  * case1: `ssh client -> bsck client -> bsck server -> ssh server` for solving ssh connect lost frequently and ssh once to inner server.
  * case2: `browser -> bsck client -> bsck server -> web server` for browser to inner web server.
* provider bash access on each node.
* provider webdav access on each node.
* provider remote execute command on each node.
* provider balanced socks5 server that using node in bsck router as proxy server or multi upstream socks5 server.
* provider gui to edit forward/channel configure.

## Package Reference
* `bsck` the main code to implement bond socket, it export all api to embedd bond socket in app.
* `dialer` the main code to implement dial to raw socket or other useful feature like bash/webdav.
  * `echo` run an echo server, it always is using when using `bs-ping` command
  * `cmd` start comamnd on node and pipe standard i/o as bsck connect.
  * `web` start webdav server on node and pipe it as bsck connect.
  * `tcp` dial tcp connect to other server and pipe it as bsck connect.
* `bsrouter` the app to start bond socket node, it run bsck server/client/slaver at the same time.
  * `gui` the gui app to start bond socket client, it can edit forward/channel.
* `bsconsole` the agent app to make noraml app can using bond socket.

## Install
### Manual Install
* install basic comamnd

```.sh
go get github.com/codingeasygo/bsck/bsrouter
go get github.com/codingeasygo/bsck/bsconsole
```

* install other command

```.sh
cp -f $GOPATH/src/github.com/codingeasygo/bsck/bsconsole/bs-ssh.sh $GOPATH/bin/bs-ssh
cp -f $GOPATH/src/github.com/codingeasygo/bsck/bsconsole/bs-scp.sh $GOPATH/bin/bs-scp
cp -f $GOPATH/src/github.com/codingeasygo/bsck/bsconsole/bs-sftp.sh $GOPATH/bin/bs-sftp
ln -sf $GOPATH/bin/bsconsole $GOPATH/bin/bs-ping
ln -sf $GOPATH/bin/bsconsole $GOPATH/bin/bs-state
ln -sf $GOPATH/bin/bsconsole $GOPATH/bin/bs-bash
ln -sf $GOPATH/bin/bsconsole $GOPATH/bin/bs-sh
```

### Install from binary
* download binary from [releases](releases)
* uncompress to `xx`
* `cd xx/bsrouter`
* `./bsrouter-install.sh -i -c` for client or `./bsrouter-install.sh -i -s` for service

## Command Reference
* `bsrouter` start bond socket server/client/slaver by configure, it will auto scan configure ordered by `args`,`./.bsrouter.json"`,`./bsrouter.json`,`HOME/.bsrouter/bsrouter.json`,`HOME/.bsrouter.json`,`/etc/bsrouter/bsrouter.json`,`/etc/bsrouer.json`
* `bsconsole` the node agent command, it will auto scan configure ordered like `bsrouter`
  * `bsconsole 'node1->tcp://cmd?exec=bash'` connect to node bash
  * `bsconsole 'node1->tcp://cmd?exec=xxx'` start xxx command on node1
  * `bsconsole -win32 'node1->tcp://cmd?exec=cmd'` start window cmd on node1
* `bs-bash <node router>` connect to node bash, equal to `bsconsole 'node1->tcp://cmd?exec=bash'`
* `bs-sh <node router>` connect to node sh, equal to `bsconsole 'node1->tcp://cmd?exec=sh'`
* `bs-ping <node router>` ping to node
  * `bs-ping node1` ping to node1
  * `bs-ping 'node1->nodex'` ping to nodex by router `node1->nodex`
* `bs-state <node router>` list node state
  * `bs-state node1` list node1 state
  * `bs-state 'node1->nodex'` list nodex state
* `bs-ssh <bsck uri> <ssh options>` start ssh connect
  * `bs-ssh 'node1->tcp://xxx:22' -lroot` start connect ssh server which after node1
* `bs-sftp <bsck uri> <ssh options>` start sftp connect
  * `bs-sftp 'node1->tcp://xxx:22' -lroot` start sftp to ssh server which after node1
* `bs-scp <bsck uri> <scp options>` start scp connect
  * `bs-scp 'node1->tcp://xxx:22' xxx root@bshost:/tmp/` copy xxx file to server after node1, the `bshost` will be auto repeaced.


## Configure
### configure file reference
* `name` the node name
* `listen` the node listen port.
* `cert`,`key` the ssl cert
* `dialer` the raw connect dialer configure.
  * `std` enable all standard dialer by `1`, it container `cmd`,`echo`,`web`,`tcp` dialer. if only want enable some dialer, can be

  ```.json
  {
    "dialer": {
        "cmd": {
            "PS1": "xxx"
        },
        "echo":{

        }
    }
  }
  ```
  * see [Dialer Reference](#dialer-reference) for more.
* `acl` the access control on bsck server
* `web` listen web and websocket on address, it always is used by `bsconsole`
* `socks5` listen socks5 on address, it always is used by `bsconsole` or socks5 balance server.

### bsck server
* generate ssl cert by

```.sh
openssl req -new -nodes -x509 -out "/etc/bsrouter/bsrouter.pem" -keyout "/etc/bsrouter/bsrouter.key" -days 3650 -subj "/C=CN/ST=NRW/L=Earth/O=Random Company/OU=IT/CN=xxx/emailAddress=cert@xxxx"
```

* edit configure on `/etc/bsrouter/bsrouter.json`

```.json
{
    "name": "server1",
    "listen": ":12023",
    "cert": "/etc/bsrouter/bsrouter.pem",
    "key": "/etc/bsrouter/bsrouter.key",
    "dialer": {
        "std": 1
    },
    "acl": {
        "slave1": "1111",
        "client1": "2222"
    },
    "showlog": 0,
    "logflags": 16
}
```

### bsck slaver

* generate ssl cert by

```.sh
openssl req -new -nodes -x509 -out "/etc/bsrouter/bsrouter.pem" -keyout "/etc/bsrouter/bsrouter.key" -days 3650 -subj "/C=CN/ST=NRW/L=Earth/O=Random Company/OU=IT/CN=xxx/emailAddress=cert@xxxx"
```

* edit configure on `/etc/bsrouter/bsrouter.json`

```.json
{
    "name": "slave1",
    "cert": "/etc/bsrouter/bsrouter.pem",
    "key": "/etc/bsrouter/bsrouter.key",
    "dialer": {
        "std": 1
    },
    "channels": [
        {
            "enable": true,
            "token": "1111",
            "local": "",
            "remote": "xxx:12023",
            "index": 0
        }
    ],
    "showlog": 0,
    "logflags": 16
}
```

### bsck client

* generate ssl cert by

```.sh
openssl req -new -nodes -x509 -out "$HOME/.bsrouter/bsrouter.pem" -keyout "$HOME/.bsrouter/bsrouter.key" -days 3650 -subj "/C=CN/ST=NRW/L=Earth/O=Random Company/OU=IT/CN=xxx/emailAddress=cert@xxxx"
```

* edit configure on `$HOME/.bsrouter/bsrouter.json`

```.json
{
    "name": "client1",
    "cert": "/xx/.bsrouter/bsrouter.pem",
    "key": "/xx/.bsrouter/bsrouter.key",
    "web": {
        "auth": "",
        "listen": ":12024",
        "suffix": ""
    },
    "channels": [
        {
            "enable": true,
            "token": "2222",
            "local": "",
            "remote": "xxx:12023",
            "index": 0
        }
    ],
    "showlog": 0,
    "logflags": 16
}
```

### testing (optional)
* `bs-ping server1` test connect to server1 (optional)

```.txt
bsconsole using config /xxx/.bsrouter/bsrouter.json
64 Bytes from server1->tcp://echo time=9.600463ms
64 Bytes from server1->tcp://echo time=5.248072ms
```

* `bs-ping 'server1->slaver1'` test connect to slaver1 (optional)

```.txt
bsconsole using config /xxx/.bsrouter/bsrouter.json
64 Bytes from server1->slaver1->tcp://echo time=11.600463ms
64 Bytes from server1->slaver1->tcp://echo time=12.248072ms
```

* `bs-state` get client1 state when do ping (optional)

```.txt
bsconsole using config /Users/vty/.bsrouter/bsrouter.json
[Channels]
 ->server1
   0    5   2018-10-24 11:30:33   channel{name:server1,index:0,cid:44,info:x.x.x.x:12023}


[Table]
 channel{name:server1,index:0,cid:44,info:x.x.x.x:12023} 52 <-> raw{uri:tcp://echo,router:server1->tcp://echo,info:ws://:12024} 52
```

* `bsconsole server1` start bash on server1 (optional)

```.txt
bsconsole using config /xxx/.bsrouter/bsrouter.json
[bsrouter@xx ~]$
```

* `bs-ssh 'server1->slaver1->tcp://x.x.x.x:22' -lroot` test connect to ssh server (optional)

### configure port forward permanent
* edit `/xx/.bsrouter/bsrouter.json` and add `forwards` configure.

```.json
{
    "name": "client1",
    "cert": "/xx/.bsrouter/bsrouter.pem",
    "key": "/xx/.bsrouter/bsrouter.key",
    "web": {
        "auth": "",
        "listen": ":12024",
        "suffix": ""
    },
    "channels": [
        {
            "enable": true,
            "token": "2222",
            "local": "",
            "remote": "xxx:12023",
            "index": 0
        }
    ],
    "forwards": {
        "test1~ws://": "server1->tcp://host1:22",
        "test1-1~tcp://localhost:10022": "server1->slaver1->tcp://host1:22",
        "test2~vnc://localhost": "server1->slaver1->tcp://host2:5900",
        "test2-1~tcp://localhost:15900": "server1->slaver1->tcp://host2:5900",
        "test3~rdp://localhost": "server1->slaver1->tcp://host3:3389"
        "test3-1~tcp://localhost:13389": "server1->slaver1->tcp://host3:3389"
        "test4~ws://": "server1->slaver1->tcp://cmd?exec=ping www.google.com"
    },
    "rdp_dir": "/tmp/bsrouter/",
    "vnc_dir": "/tmp/bsrouter/",
    "showlog": 0,
    "logflags": 16
}
```

* in this case, bsck foward ssh/vnc/rdp
  * `bs-ssh test1 -lroot` to ssh connect host1
  * `ssh -p 10022 localhost -lroot` to ssh connect host1
  * connect host2 by vnc client: open vnc client then open `test2.vnc` file on `/tmp/bsrouter/`
  * connect host2 by vnc client: open vnc client then using localhost:15900 to connect
  * connect host3 by rdp client: open rdp client then open `test3.rdp` file on `/tmp/bsrouter/`
  * connect host3 by rdp client: open rdp client then using localhost:13389 to connect
  * `bsconsole test4` execute ping on slaver1

* more for forward configure, see [Forward Reference]($forward-reference)

### configure balance socks5 proxy
* edit `/xx/.bsrouter/bsrouter.json` and add `proxy` configure.

```.json
{
    "name": "client1",
    "cert": "/xx/.bsrouter/bsrouter.pem",
    "key": "/xx/.bsrouter/bsrouter.key",
    "web": {
        "auth": "",
        "listen": ":12024",
        "suffix": ""
    },
    "socks5": ":11081",
    "channels": [
        {
            "enable": true,
            "token": "2222",
            "local": "",
            "remote": "xxx:12023",
            "index": 0
        }
    ],
    "proxy": {
        "id": "proxy",
        "type": "balance",
        "matcher": "^.*://.*$",
        "dialers": [
            {
                "id": "r1",
                "type": "router",
                "router": "server1->${URI}"
            },
            {
                "id": "r2",
                "type": "router",
                "router": "server1->slaver1->${URI}"
            },
            ,
            {
                "id": "r3",
                "type": "socks",
                "address": "xxx:xxx"
            }
        ],
        "filter": [
            {
                "matcher": "^.*x1.*$",
                "access": 1
            },
            {
                "matcher": "^.*x2.*$",
                "access": 1
            },
            {
                "matcher": "^.*$",
                "access": 0
            }
        ],
        "policy": [
            {
                "matcher": "^.*x1.*$",
                "limit": [
                    3000,
                    10
                ]
            },
            {
                "matcher": "^.*x2.*$",
                "limit": [
                    3000,
                    10
                ]
            }
        ]
    },
    "showlog": 0,
    "logflags": 16
}
```

* in this case, havin two proxy dialer in two router `server`,`slaver1` and one upstream proxy, only host contain x1,x2 can connect, and limit x1,x2 connect count by 10 time per 3000ms. this case is always used on crawler.

## Dialer Reference
### `cmd`
```.json
{
    "dialer": {
        "cmd": {
            "PS1": "bsrouter",
            "Dir": "/tmp",
            "Env":{
                "a": 1,
                "b": 2
            }
        }
    }
}
```
* `PS1` the shell ps1
* `Dir` the start directroy
* `LC` the i/o encoding.
* `Env` the enviroment values
* `reuse_timeout` the reuse session timeout.

### `echo`

```.json
{
    "dialer": {
        "echo": {}
    }
}
```

### `socks`

```.json
{
    "dialer": {
        "socks": {
            "id": "s1",
            "address": "xxx:xx",
            "matcher": "^.*$"
        }
    }
}
```

* `id` the dialer id (required)
* `address` the socks5 server address (required)
* `matcher` match uri to access to connect. (optional)

### `web`

```.json
{
    "dialer": {
        "web": {}
    }
}
```

### `tcp`

```.json
{
    "dialer": {
        "tcp": {
            "bind": "xxxx:xx"
        }
    }
}
```

* `bind` bind to local address before connect to remote.


## Forward Reference

### remote uri
the remote uri scheme is `node1->node1->..->nodex->protocol://host:port?arg=val`

supported protocol

* `tcp://host:port?arg=val` normal tcp dialer, the arguments
  * `bind` bind to local address before connect to remote (optional)
* `tcp://cmd?arg=val` execute command on node
  * `exec` the command and command argument to exec (required)
  * `LC` the i/o encoding
  * `reuse` enable/disable reuse session, 1 is enable, 0 is disable.
* `tcp://echo` start echo server
* `http://web` start web server on node
  * `dir` the webdav work directory.

### local uri
the local uri scheme is `alias~protocol://user:password@host:port`, the alias can be used on `bsconsole`,`bs-sftp`,`bs-scp`,`bs-ssh`

supported protocol

* `alias~tcp://host:port` listen tcp by host:port
* `alias~socks://host:port` listen socks5  by host:port,
* `alias~rdp://user@host:port` listen tcp  by host:port, and generate rdp file on `rdp_dir` by alias.rdp, password is not supported by rdp file
* `alias~vnc://:passwrod@host:port` listen tcp  by host:port, and generate rdp file on `vnc_dir` by alias.vnc, user is not needed, password is encrypted
* `alias~web://` foward web by `http://localhost:port/dav/alias` to uri when `web` configure is enabled.
* `alias~ws://` foward websocekt by `ws://localhost:port/ws/alias` to uri when `web` configure is enabled.

## GUI Reference

* generate ssl cert by

```.sh
openssl req -new -nodes -x509 -out "./bsrouter.pem" -keyout "./bsrouter.key" -days 3650 -subj "/C=CN/ST=NRW/L=Earth/O=Random Company/OU=IT/CN=xxx/emailAddress=cert@xxxx"
```

* copy `bsrouter.pem`,`bsrouter.key` to `~/.bsrouter/`

* open GUI program and edit `basic`,`channels`,`forwards`