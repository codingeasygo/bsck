require('source-map-support').install();
import { BrowserWindow, app, ipcMain, session, dialog } from "electron"
import * as path from "path"
import * as log4js from "log4js"
const Log = log4js.getLogger("main")

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
    mainWindow = new BrowserWindow({
        width: 1024,
        height: level == "debug" ? 768 : 500,
        frame: level == "debug"
    })
    // mainWindow.setIgnoreMouseEvents(true)
    // mainWindow.setMenuBarVisibility(false)
    if (level == "debug") {
        mainWindow.webContents.openDevTools()
    }
    mainWindow.loadFile(`dist/bsrouter/index.html`)
    // mainWindow.loadFile(`/tmp/t.html`)
    // mainWindow.loadURL("http://www.baidu.com")
    // mainWindow.on("close", (e: Event) => {
    // })
    // mainWindow.on('closed', (e: Event) => {
    //     mainWindow = null
    // })
}

app.on('ready', () => {
    createWindow()
})

app.on('window-all-closed', () => {
    app.quit()
})

app.on('activate', () => {
    if (mainWindow === null) {
        createWindow()
    }
})

app.on("before-quit", (e) => {
});

