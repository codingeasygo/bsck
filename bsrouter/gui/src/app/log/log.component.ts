import { Component, OnInit, Input, ViewChild, ElementRef } from '@angular/core';
import { BsrouterService } from '../bsrouter.service';

@Component({
  selector: 'app-log',
  templateUrl: './log.component.html',
  styleUrls: ['./log.component.css']
})
export class LogComponent implements OnInit {
  @Input() set activated(v: boolean) {
  }
  @ViewChild("log") log: ElementRef
  srv: BsrouterService;
  constructor(srv: BsrouterService) {
    this.srv = srv;
  }
  ngOnInit() {
    this.srv.bsrouterHandler.subscribe(n => {
      if (n.cmd == "log") {
        this.log.nativeElement.innerText += n.m;
      }
    })
  }
}
