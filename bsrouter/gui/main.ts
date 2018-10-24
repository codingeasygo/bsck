require('source-map-support').install();
import { BrowserWindow, app, Menu, Tray, MenuItemConstructorOptions, ipcMain, MenuItem } from "electron"
import * as path from "path"
import * as log4js from "log4js"
import * as os from "os"
import * as fs from "fs"
import * as url from "url"
import { spawn } from "child_process";
const Log = log4js.getLogger("main")

const homedir = os.homedir();

class Bsrouter {
    public homeDir = homedir;
    public workingFile: string = os.homedir() + "/.bsrouter/bsrouter.json";
    public handler: BsrouterHandler
    public status: string = "Stopped";
    private runner: any;
    private restarting: boolean
    constructor() {
    }
    public start() {
        if (this.status == "Running") {
            return "Running";
        }
        Log.info("bsrouter is starting")
        this.runner = spawn(__dirname + '/bsrouter');
        this.runner.stdout.on('data', (data) => {
            if (this.handler) {
                this.handler.onLog(data.toString());
            }
            if (this.status != "Running") {
                this.status = "Running";
                this.handler.onStatus(this.status);
            }
        });
        this.runner.stderr.on('data', (data) => {
            if (this.handler) {
                this.handler.onLog(data.toString())
            }
            if (this.status != "Running") {
                this.status = "Running";
                this.handler.onStatus(this.status);
            }
        });
        this.runner.on('exit', (code) => {
            if (this.handler) {
                this.handler.onLog(`child process exited with code ${code}` + "\n");
            }
            this.status = "Stopped";
            this.handler.onStatus(this.status);
            if (this.restarting) {
                setTimeout(() => this.start(), 1000)
            }
        });
        this.runner.on('error', (e) => {
            if (this.handler) {
                this.handler.onLog(`child process error with ${e}` + "\n");
            }
            this.status = "Error"
            this.handler.onStatus(this.status);
        });
        this.status = "Pending"
        this.handler.onStatus(this.status);
        return "OK"
    }
    public stop() {
        if (this.status == "Stopped") {
            return "Stopped"
        }
        Log.info("bsrouter is stopping")
        this.runner.kill()
        return "OK"
    }
    public restart() {
        if (this.status == "Stopped") {
            return "Stopped"
        }
        Log.info("bsrouter is restarting")
        this.restarting = true
        this.stop()
        return "OK"
    }
    public loadConf(): any {
        try {
            var data = fs.readFileSync(this.workingFile)
            return JSON.parse(data.toString());
        } catch (e) {
            return {};
        }
    }
    protected saveConf(conf: any) {
        let dir = path.dirname(this.workingFile);
        if (!fs.existsSync(dir)) {
            fs.mkdirSync(dir)
        }
        fs.writeFileSync(this.workingFile, JSON.stringify(conf));
    }
    public loadBasic(): any {
        var conf = this.loadConf();
        if (conf) {
            delete conf.forwards;
            delete conf.channels;
        }
        return conf;
    }
    public saveBasic(basic: any) {
        delete basic.forwards;
        delete basic.channels;
        var old = this.loadConf();
        Object.assign(old, basic);
        this.saveConf(old);
        return "OK"
    }
    public loadForwards(): any {
        let fs = this.loadConf().forwards;
        if (!fs) {
            fs = {};
        }
        return fs;
    }
    public addForward(key: string, router: string) {
        var old = this.loadConf();
        if (!old.forwards) {
            old.forwards = {};
        }
        old.forwards[key] = router;
        this.saveConf(old);
        return "OK"
    }
    public removeForward(key: string) {
        var old = this.loadConf();
        if (!old.forwards) {
            old.forwards = {}
        }
        delete old.forwards[key]
        this.saveConf(old);
        return "OK"
    }
    public loadChannels(): any[] {
        let cs = this.loadConf().channels;
        if (!cs) {
            cs = [];
        }
        return cs;
    }
    public addChannel(c: any) {
        var old = this.loadConf();
        old.channels.push(c)
        this.saveConf(old);
        return "OK"
    }
    public removeChannel(i: number) {
        var old = this.loadConf();
        old.channels.splice(i, 1);
        this.saveConf(old);
        return "OK"
    }
    public enableChannel(i: number, enabled: any) {
        var old = this.loadConf();
        old.channels[i].enable = enabled && true;
        this.saveConf(old);
        return "OK"
    }
}

export interface BsrouterHandler {
    onLog(m: string)
    onStatus(s: string)
}


let bsrouter = new Bsrouter()

let mainWindow: BrowserWindow
function createWindow() {
    let level = "info"
    if (process.argv.length > 2) {
        level = process.argv[2]
    }
    log4js.configure({
        appenders: {
            ruleConsole: { type: 'console' },
            ruleFile: {
                type: 'dateFile',
                filename: path.join(app.getPath('userData'), "logs", 'gambling'),
                pattern: '-yyyy-MM-dd.log',
                maxLogSize: 100 * 1024 * 1024,
                numBackups: 30,
                alwaysIncludePattern: true
            }
        },
        categories: {
            default: { appenders: ['ruleConsole', 'ruleFile'], level: level }
        },
    });
    let tray = new Tray(__dirname + '/view/assets/stopped@4x.png')
    tray.setToolTip('This is BSRouter')
    mainWindow = new BrowserWindow({
        width: level == "debug" ? 1500 : 1024,
        height: level == "debug" ? 518 : 500,
        frame: level == "debug",
        title: "bsrouter",
    })
    if (level == "debug") {
        mainWindow.webContents.openDevTools()
    }
    mainWindow.loadFile(`dist/view/index.html`)
    function callOpen(f: string) {
        try {
            let conf = bsrouter.loadConf();
            var dir = conf.vnc_dir;
            if (!dir) {
                dir = os.homedir() + "/Desktop";
            }
            let u = url.parse(f);
            switch (u.protocol) {
                case "vnc:":
                case "locvnc:":
                    spawn("open", [dir + "/" + u.hostname + ".vnc"])
                    break
                case "rdp:":
                case "locrdp:":
                    spawn("open", [dir + "/" + u.hostname + ".rdp"])
                    break
            }
            return "OK"
        } catch (e) {
            return "" + e
        }
    }
    function clickMenu(menuItem: MenuItem, browserWindow: BrowserWindow, event: Event) {
        let menu = menuItem as any;
        callOpen(menu.id)
    }
    function reloadMenu() {
        let menus: MenuItemConstructorOptions[] = []
        menus.push(
            { label: 'BSRouter ' + bsrouter.status, type: 'normal', enabled: false },
            {
                label: 'BSRouter Start/Stop', type: 'normal', click: () => {
                    if (bsrouter.status == "Stopped") {
                        bsrouter.start()
                    } else if (bsrouter.status == "Running") {
                        bsrouter.stop()
                    }
                }
            },
            {
                label: 'BSRouter Restart', type: 'normal', click: () => {
                    bsrouter.restart()
                }
            },
        )
        let vncs: MenuItemConstructorOptions[] = []
        let rdps: MenuItemConstructorOptions[] = []
        let conf = bsrouter.loadConf()
        if (conf.forwards) {
            for (let k in conf.forwards) {
                let u = url.parse(k);
                switch (u.protocol) {
                    case "vnc:":
                    case "locvnc:":
                        vncs.push({ label: "open " + u.hostname + ".vnc", type: 'normal', click: clickMenu, id: k })
                        break
                    case "rdp:":
                    case "locrdp:":
                        rdps.push({ label: "open " + u.hostname + ".rdp", type: 'normal', click: clickMenu, id: k })
                        break
                }
            }
        }
        if (vncs.length) {
            menus.push({ type: 'separator' }, { label: 'VNC', type: 'normal', enabled: false })
            menus.push(...vncs)
        }
        if (rdps.length) {
            menus.push({ type: 'separator' }, { label: 'RDP', type: 'normal', enabled: false })
            menus.push(...rdps)
        }
        menus.push(
            { type: 'separator' },
            {
                label: 'Configure', type: 'normal', click: () => {
                    mainWindow.show();
                }
            },
            {
                label: 'Quit', type: 'normal', click: () => {
                    app.quit()
                }
            }
        )
        tray.setContextMenu(Menu.buildFromTemplate(menus))
    }
    bsrouter.handler = {
        onLog: (m) => {
            // Log.info("%s", m.replace("\n", ""));
            mainWindow.webContents.send("log", m)
        },
        onStatus: (s) => {
            mainWindow.webContents.send("status", s)
            reloadMenu()
            if (s == "Running") {
                tray.setImage(__dirname + '/view/assets/running@4x.png')
            } else {
                tray.setImage(__dirname + '/view/assets/stopped@4x.png')
            }
        }
    }
    ipcMain.on("hideConfigure", () => {
        mainWindow.hide()
    })
    ipcMain.on("startBsrouter", (e) => {
        e.returnValue = bsrouter.start()
    })
    ipcMain.on("stopBsrouter", (e) => {
        e.returnValue = bsrouter.stop()
    })
    ipcMain.on("statusBsrouter", (e) => {
        e.returnValue = bsrouter.start()
    })
    ipcMain.on("loadBasic", (e) => {
        e.returnValue = bsrouter.loadBasic()
    })
    ipcMain.on("saveBasic", (e, args) => {
        e.returnValue = bsrouter.saveBasic(args)
    })
    ipcMain.on("loadForwards", (e) => {
        e.returnValue = bsrouter.loadForwards()
    })
    ipcMain.on("addForward", (e, args) => {
        e.returnValue = bsrouter.addForward(args.key, args.router)
    })
    ipcMain.on("removeForward", (e, args) => {
        e.returnValue = bsrouter.removeForward(args.key)
    })
    ipcMain.on("openForward", (e, args) => {
        e.returnValue = callOpen(args)
    })
    ipcMain.on("loadChannels", (e) => {
        e.returnValue = bsrouter.loadChannels()
    })
    ipcMain.on("addChannel", (e, args) => {
        e.returnValue = bsrouter.addChannel(args)
    })
    ipcMain.on("removeChannel", (e, args) => {
        e.returnValue = bsrouter.removeChannel(args)
    })
    ipcMain.on("enableChannel", (e, args) => {
        e.returnValue = bsrouter.enableChannel(args.index, args.enabled)
    })
    reloadMenu()
    const template: MenuItemConstructorOptions[] = [
        {
            label: 'Edit',
            submenu: [
                { role: 'undo' },
                { role: 'redo' },
                { type: 'separator' },
                { role: 'cut' },
                { role: 'copy' },
                { role: 'paste' },
                { role: 'pasteandmatchstyle' },
                { role: 'delete' },
                { role: 'selectall' }
            ]
        },
        {
            role: 'window',
            submenu: [
                { role: 'minimize' },
                { role: 'close' }
            ]
        }
    ]
    const menu = Menu.buildFromTemplate(template)
    Menu.setApplicationMenu(menu)
}

app.on('ready', () => {
    createWindow()
})

app.on('window-all-closed', () => {
})

app.on('activate', () => {
    if (mainWindow === null) {
        createWindow()
    }
})

app.on("before-quit", (e) => {
    try {
        bsrouter.handler = {
            onLog: (m) => { },
            onStatus: (s) => { }
        };
        bsrouter.stop()
    } catch (e) {
    }
});

