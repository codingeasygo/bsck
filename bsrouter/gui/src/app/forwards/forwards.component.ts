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
  forward: any = { protocol: "ws" }
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
    let fs = this.srv.loadForwards()
    let forwards = [];
    for (let k in fs) {
      let parts = k.split("~");
      if (parts.length < 2) {
        continue;
      }
      let u = url.parse(parts[1]);
      let f: any = {};
      f.protocol = u.protocol.replace(":", "");
      f.port = u.port;
      f.host = u.hostname;
      f.address = "";
      if (u.hostname) {
        f.address = u.hostname
      }
      if (u.port) {
        f.address += ":" + u.port;
      }
      f.name = parts[0];
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
    forwards.sort((a, b) => {
      if (a.name == b.name) {
        return 0;
      }
      if (a.name < b.name) {
        return -1;
      } else {
        return 1;
      }
    })
    this.allForwards = forwards;
  }
  remove(f: any) {
    this.srv.removeForward(f.k);
    this.reload();
  }
  add() {
    if (!this.forward.protocol || !this.forward.name || !this.forward.router) {
      this.showError = true;
      return;
    }
    this.showError = false;
    let key = "";
    key += this.forward.name + "~";
    key += this.forward.protocol + "://";
    if (this.forward.username) {
      key += this.forward.username;
    }
    if (this.forward.password) {
      key += ":" + this.forward.password;
    }
    if (this.forward.username || this.forward.password) {
      key += "@";
    }
    if (this.forward.address) {
      key += this.forward.address;
    } else if (this.forward.protocol != "ws" && this.forward.protocol != "web") {
      key += "localhost";
    }
    this.srv.addForward(key, this.forward.router);
    this.reload();
    this.forward = { protocol: "ws" };
  }
  open(f) {
    this.srv.openForward(f.k)
  }
}
