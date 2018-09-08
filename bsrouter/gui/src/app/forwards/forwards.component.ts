import { Component, OnInit, Input } from '@angular/core';
import { BsrouterService } from '../bsrouter.service';
declare var url: any;

@Component({
  selector: 'app-forwards',
  templateUrl: './forwards.component.html',
  styleUrls: ['./forwards.component.css']
})
export class ForwardsComponent implements OnInit {
  srv: BsrouterService;
  allForwards: any = []
  forward: any = {}
  showError: boolean = false;
  @Input() set activated(v: boolean) {
  }
  constructor(srv: BsrouterService) {
    this.srv = srv;
  }

  ngOnInit() {
    this.reload();
  }

  reload() {
    try {
      let fs = this.srv.loadForwards()
      let forwards = [];
      for (let k in fs) {
        let u = url.parse(k);
        let f: any = {};
        f.protocol = u.protocol.replace(":", "");
        f.port = u.port;
        f.name = u.hostname;
        if (u.auth) {
          let auth = u.auth.split(":", 2)
          f.username = auth[0]
          if (auth.length > 1) {
            f.password = auth[1]
          }
        }
        f.k = k;
        f.router = fs[k];
        forwards.push(f);
      }
      this.allForwards = forwards;
    } catch (e) {
      window.alert(e)
    }
  }
  remove(f: any) {
    try {
      this.srv.removeForward(f.k);
      this.reload();
    } catch (e) {
      window.alert(e)
    }
  }
  add() {
    try {
      if (!this.forward.protocol || !this.forward.name || !this.forward.router) {
        this.showError = true;
        return;
      }
      this.showError = false;
      let key = "";
      key += this.forward.protocol + "://";
      if (this.forward.username) {
        key += this.forward.username;
      }
      if (this.forward.password) {
        key += ":" + this.forward.password;
      }
      if (this.forward.username || this.forward.password) {
        key += "@"
      }
      key += this.forward.name;
      if (this.forward.port) {
        key += ":" + this.forward.port;
      }
      this.srv.addForward(key, this.forward.router);
      this.reload();
      this.forward = {};
    } catch (e) {
      window.alert(e)
    }
  }
  open(f) {
    try {
      let basic = this.srv.loadBasic()
      if (f.protocol == "vnc") {
        var dir = basic.vnc_dir;
        if (!dir) {
          dir = this.srv.homeDir + "/Desktop";
        }
        this.srv.openForward(dir + "/" + f.name + ".vnc");
      } else if (f.protocol == "rdp") {
        var dir = basic.vnc_dir;
        if (!dir) {
          dir = this.srv.homeDir + "/Desktop";
        }
        this.srv.openForward(dir + "/" + f.name + ".rdp");
      }
    } catch (e) {
      window.alert(e)
    }
  }
}
