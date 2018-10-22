import { Injectable } from '@angular/core';
import { Subject } from 'rxjs';
declare var ipcRenderer: any;

@Injectable({
  providedIn: 'root'
})
export class BsrouterService {
  public bsrouterHandler = new Subject<any>()
  constructor() {
    ipcRenderer.on("log", (e, m) => {
      this.bsrouterHandler.next({ cmd: "log", m: m })
    })
    ipcRenderer.on("status", (e, m) => {
      this.bsrouterHandler.next({ cmd: "status", status: m })
    })
  }
  public startBsrouter() {
    return ipcRenderer.sendSync("startBsrouter", {})
  }
  public stopBsrouter() {
    return ipcRenderer.sendSync("stopBsrouter", {})
  }
  public loadBasic(): any {
    return ipcRenderer.sendSync("loadBasic", {})
  }
  public saveBasic(basic: any) {
    delete basic.forwards;
    delete basic.channels;
    return ipcRenderer.sendSync("saveBasic", basic)
  }
  public loadForwards(): any {
    return ipcRenderer.sendSync("loadForwards", {})
  }
  public addForward(key: string, router: string) {
    return ipcRenderer.sendSync("addForward", { key: key, router: router })
  }
  public removeForward(key: string) {
    return ipcRenderer.sendSync("removeForward", { key: key })
  }
  public openForward(forward: string) {
    return ipcRenderer.sendSync("openForward", forward)
  }
  public loadChannels(): any[] {
    return ipcRenderer.sendSync("loadChannels", {})
  }
  public addChannel(c: any) {
    return ipcRenderer.sendSync("addChannel", c)
  }
  public removeChannel(i: number) {
    return ipcRenderer.sendSync("removeChannel", i)
  }
  public enableChannel(i: number, enabled: boolean) {
    return ipcRenderer.sendSync("enableChannel", { index: i, enabled: enabled })
  }
}