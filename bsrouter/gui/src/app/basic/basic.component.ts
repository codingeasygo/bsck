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
    try {
      var c = Object.assign({}, this.conf);
      c.showlog = c.showlog ? 1 : 0;
      console.log("saving config ", c);
      this.srv.saveBasic(c);
      this.showMessage("saved");
    } catch (e) {
      console.error("saving config fail with ", e)
      this.showMessage("save fail");
    }
  }
  showMessage(m: string) {
    this.message = m;
    setTimeout(() => this.message = "", 4000);
  }
}
