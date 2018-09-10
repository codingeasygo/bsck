import { Component, OnInit, Input } from '@angular/core';
import { BsrouterService } from '../bsrouter.service';

@Component({
  selector: 'app-basic',
  templateUrl: './basic.component.html',
  styleUrls: ['./basic.component.css']
})
export class BasicComponent implements OnInit {
  srv: BsrouterService;
  conf: any = {
    web: {},
  }
  message: string = ""
  dimissDelay: number = 4000
  @Input() set activated(v: boolean) {
  }
  constructor(srv: BsrouterService) {
    this.srv = srv;
  }
  ngOnInit() {
    this.reload();
  }
  reload() {
    this.conf = this.srv.loadBasic()
    this.conf.showlog = true && this.conf.showlog;
    console.log("load config ", this.conf);
  }
  save() {
    let c = Object.assign({}, this.conf);
    c.showlog = c.showlog ? 1 : 0;
    console.log("saving config ", c);
    let res = this.srv.saveBasic(c);
    if (res == "OK") {
      this.showMessage("saved");
    } else {
      this.showMessage("save fail by " + res);
    }
  }
  showMessage(m: string) {
    this.message = m;
    setTimeout(() => this.message = "", this.dimissDelay);
  }
}
