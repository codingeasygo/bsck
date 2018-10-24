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
* provider remove execute command on each node.
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

```.go
go get github.com/sutils/bsck/bsrouter
go get github.com/sutils/bsck/bsconsole
```

* install other command

```
cp -f $GOPATH/src/github.com/sutils/bsck/bsconsole/bs-ssh.sh $GOPATH/bin/bs-ssh
cp -f $GOPATH/src/github.com/sutils/bsck/bsconsole/bs-scp.sh $GOPATH/bin/bs-scp
cp -f $GOPATH/src/github.com/sutils/bsck/bsconsole/bs-sftp.sh $GOPATH/bin/bs-sftp
ln -sf $GOPATH/bin/bsconsole $GOPATH/bin/bs-ping
ln -sf $GOPATH/bin/bsconsole $GOPATH/bin/bs-state
```

### Install from binary
* download binary from release
* uncompress to `xx`
* `cd xx/bsrouter`
* `./bsrouter-install.sh -i -c` for client or `./bsrouter-install.sh -i -s` for service

## Command Reference
* `bsrouter` start bond socket server/client/slaver by configure, it will auto scan configure ordered by `args`,`./.bsrouter.json"`,`./bsrouter.json`,`HOME/.bsrouter/bsrouter.json`,`HOME/.bsrouter.json`,`/etc/bsrouter/bsrouter.json`,`/etc/bsrouer.json`
* `bsconsole` the node agent command, it will auto scan configure ordered like `bsrouter`
  * `bsconsole <node name>` connect to node bash
  * `bsconsole 'node1->tcp://cmd?exec=xxx'` start xxx command on node1
  * `bsconsole -win32 'node1->tcp://cmd?exec=cmd'` start window cmd on node1
* `bs-ping <node router>` ping to node
  * `bs-ping node1` ping to node1
  * `bs-ping node1->nodex` ping to nodex by router `node1->nodex`
* `bs-state <node router>` list node state
  * `bs-state node1` list node1 state
  * `bs-state node1->nodex` list nodex state
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
        "ws://test1": "server1->tcp://host1:22",
        "tcp://test1-1:10022": "server1->slaver1->tcp://host1:22",
        "vnc://test2": "server1->slaver1->tcp://host2:5900",
        "tcp://test2-1:15900": "server1->slaver1->tcp://host2:5900",
        "rdp://test3": "server1->slaver1->tcp://host3:3389"
        "tcp://test3-1:13389": "server1->slaver1->tcp://host3:3389"
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
  * connect host2 by rdp client: open rdp client then using localhost:13389 to connect

* more for forward configure, see [Forward Reference]($forward-reference)


## Dialer Reference


## Forward Reference

