import { Injectable } from '@angular/core';
import { Subject } from 'rxjs';
declare var fs: any;
declare var os: any;
declare var spawn: any;
declare var process: any;
declare var __dirname: any;
const homedir = os.homedir();

@Injectable({
  providedIn: 'root'
})
export class BsrouterService {
  public homeDir = homedir;
  public workingFile: string = homedir + "/.bsrouter/bsrouter.json";
  public bsrouterHandler = new Subject<any>()
  bsrouterStatus: string;
  bsrouter: any;
  constructor() {
    window.onunload = () => {
      if (this.bsrouter) {
        this.bsrouter.kill();
      }
    }
  }
  public startBsrouter() {
    if (this.bsrouterStatus == "Started") {
      throw "Started";
    }
    this.bsrouter = spawn(__dirname + '/../bsrouter');
    this.bsrouter.stdout.on('data', (data) => {
      if (this.bsrouterHandler) {
        this.bsrouterHandler.next({ cmd: "log", m: data.toString() });
      }
      if (this.bsrouterStatus != "Running") {
        this.bsrouterStatus = "Running"
        this.bsrouterHandler.next({ cmd: "status", status: this.bsrouterStatus });
      }
    });
    this.bsrouter.stderr.on('data', (data) => {
      if (this.bsrouterHandler) {
        this.bsrouterHandler.next({ cmd: "log", m: data.toString() });
      }
      if (this.bsrouterStatus != "Running") {
        this.bsrouterStatus = "Running"
        this.bsrouterHandler.next({ cmd: "status", status: this.bsrouterStatus });
      }
    });
    this.bsrouter.on('exit', (code) => {
      if (this.bsrouterHandler) {
        this.bsrouterHandler.next({ cmd: "log", m: `child process exited with code ${code}` });
      }
      this.bsrouterStatus = "Stopped"
      this.bsrouterHandler.next({ cmd: "status", status: this.bsrouterStatus });
    });
    this.bsrouter.on('error', (e) => {
      if (this.bsrouterHandler) {
        this.bsrouterHandler.next({ cmd: "log", m: `child process error with ${e}` });
      }
      this.bsrouterStatus = "Error"
      this.bsrouterHandler.next({ cmd: "status", status: this.bsrouterStatus });
    });
    // this.bsrouterStatus = "Pending"
    // this.bsrouterHandler.next({ cmd: "status", status: this.bsrouterStatus });
  }
  public stopBsrouter() {
    if (this.bsrouterStatus == "Stopped") {
      throw "Stopped";
    }
    this.bsrouter.kill()
  }
  public loadConf(): any {
    try {
      var data = fs.readFileSync(this.workingFile)
      return JSON.parse(data);
    } catch (e) {
      return {};
    }
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
    fs.writeFileSync(this.workingFile, JSON.stringify(old));
  }
  public loadForwards(): any {
    return this.loadConf().forwards;
  }
  public addForward(key: string, router: string) {
    var old = this.loadConf();
    if (!old.forwards) {
      old.forwards = {}
    }
    old.forwards[key] = router
    fs.writeFileSync(this.workingFile, JSON.stringify(old));
  }
  public removeForward(key: string) {
    var old = this.loadConf();
    if (!old.forwards) {
      old.forwards = {}
    }
    delete old.forwards[key]
    fs.writeFileSync(this.workingFile, JSON.stringify(old));
  }
  public openForward(file: string) {
    spawn('open', [file]);
  }
  public loadChannels(): any[] {
    return this.loadConf().channels;
  }
  public addChannel(c: any) {
    var old = this.loadConf();
    old.channels.push(c)
    fs.writeFileSync(this.workingFile, JSON.stringify(old));
  }
  public removeChannel(i: number) {
    var old = this.loadConf();
    old.channels.splice(i, 1);
    fs.writeFileSync(this.workingFile, JSON.stringify(old));
  }
}

export interface BsrouterHandler {
  onLog(m: string)
  onStatus(s: string)
}