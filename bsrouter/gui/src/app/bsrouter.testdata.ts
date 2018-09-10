

export class MockIpcRenderer {
    public fail: boolean = false;
    conf = {
        "name": "cny",
        "listen": ":2023",
        "cert": "/Users/vty/.bsrouter/bsrouter.pem",
        "key": "/Users/vty/.bsrouter/bsrouter.key",
        "web": {
            "listen": ":2024",
            "suffix": "",
            "auth": ""
        },
        "socks5": ":1081",
        "forwards": {
            "ws://dev": "MkDev->tcp://cmd?exec=/bin/bash",
            "ws://dev-network": "MkDev->tcp://192.168.1.131:22",
            "rdp://mk@dev-win-x86": "MkDev->tcp://10.211.1.103:3389",
            "vnc://:17544784f035a496@CzyVnc1": "MkIdc->UmsCzy->tcp://192.168.33.102:5900",
            "vnc://:17544784f035a496@CzyVnc2": "MkIdc->UmsCzy->tcp://192.168.33.104:5900",
            "tcp://IdcExsi@:10443": "MkIdc->tcp://192.168.2.132:443",
            "rdp://gjp@idc-win-gjp": "MkIdc->tcp://192.168.2.131:3389",
            "tcp://UmsDb:13306": "MkIdc->tcp://192.168.2.152:3306"
        },
        "channels": [
            {
                "enable": true,
                "token": "scorpion",
                "local": "",
                "remote": "rs.dyang.org:2023",
                "index": 0
            },
            {
                "enable": false,
                "token": "scorpion",
                "local": "",
                "remote": "14.23.162.172:2023",
                "index": 0
            },
            {
                "enable": false,
                "token": "scorpion",
                "local": "",
                "remote": "loc.m:2023",
                "index": 0
            }
        ],
        "dialer": {},
        "showlog": 0,
        "logflags": 16
    }
    events: any = {}
    timer: any = null
    timerc: number = 0
    public on(key, cb: () => void) {
        this.events[key] = cb
    }
    public sendSync(c, args) {
        switch (c) {
            case "startBsrouter":
                return this.startBsrouter()
            case "stopBsrouter":
                return this.stopBsrouter()
            case "loadBasic":
                let basic: any = {};
                Object.assign(basic, this.conf);
                delete basic.forwards;
                delete basic.channels;
                return basic;
            case "saveBasic":
                if (this.fail) {
                    return "mock error"
                }
                Object.assign(this.conf, args)
                return "OK"
            case "loadForwards":
                return this.conf.forwards;
            case "addForward":
                this.conf.forwards[args.key] = args.router;
                return "OK"
            case "removeForward":
                delete this.conf.forwards[args.key]
                return "OK"
            case "openForward":
                return "OK"
            case "loadChannels":
                return this.conf.channels;
            case "addChannel":
                this.conf.channels.push(args)
                return "OK"
            case "removeChannel":
                this.conf.channels.splice(args, 1)
                return "OK"

        }
    }
    public startBsrouter() {
        if (this.timer) {
            return
        }
        this.timerc = 0
        this.timer = setInterval(() => {
            if (this.timerc == 0 && this.events["status"]) {
                this.events["status"](this, "Running")
            }
            this.timerc++;
            if (this.events["log"]) {
                this.events["log"](this, `log ${this.timerc}`)
            }
        }, 100)
        this.events["status"](this, "Pending")
    }
    public stopBsrouter() {
        clearInterval(this.timer)
        this.timer = null
        this.events["status"](this, "Stopped")
    }
}

export let URL = {
    parse: (u) => {
        let vals: any = {}
        let parts = u.split("//")
        vals.protocol = parts[0]
        parts = parts[1].split("@")
        if (parts.length > 1) {
            vals.auth = parts[0]
            parts = parts[1].split(":")
        } else {
            parts = parts[0].split(":")
        }
        vals.hostname = parts[0]
        if (parts.length > 1) {
            vals.port = parts[1]
        }
        return vals
    }
}

export function sleep(delay: number): Promise<void> {
    return new Promise<void>((resolve) => {
        setTimeout(() => resolve(), delay)
    })
}