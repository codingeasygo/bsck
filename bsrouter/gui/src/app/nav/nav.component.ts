import { Component, OnInit, EventEmitter, Output, Input, ChangeDetectorRef } from '@angular/core';
import { BsrouterService } from '../bsrouter.service';

@Component({
  selector: 'app-nav',
  templateUrl: './nav.component.html',
  styleUrls: ['./nav.component.css']
})
export class NavComponent implements OnInit {
  @Output() switch = new EventEmitter<{ key: string }>();
  @Input() public activated: string = "basic"
  status: string = "Stopped"
  srv: BsrouterService;
  ref: ChangeDetectorRef
  constructor(srv: BsrouterService, ref: ChangeDetectorRef) {
    this.srv = srv;
    this.ref = ref;
  }
  ngOnInit() {
    // this.activated = ["active", "", ""]
    this.srv.bsrouterHandler.subscribe(n => {
      if (n.cmd == "status") {
        this.status = n.status;
        this.ref.detectChanges()
        console.log("bsrouter status is ", this.status);
      }
    })
  }
  doItemClick(key: string) {
    this.activated = key
    this.switch.emit({ key: key })
  }
  doTaskAction() {
    try {
      switch (this.status) {
        case "Running":
          this.srv.stopBsrouter()
          break
        case "Stopped":
        case "Error":
          this.srv.startBsrouter()
          this.status = "Pending"
          break
        default:
          break
      }
    } catch (e) {
      window.alert(e)
    }
  }
  taskStatusClass() {
    switch (this.status) {
      case "Running":
        return "nav-running"
      case "Pending":
        return "nav-pending"
      default:
        return "nav-stopped"
    }
  }
  taskActionText(): string {
    switch (this.status) {
      case "Running":
        return "Stop"
      case "Pending":
        return "Pending"
      default:
        return "Start"
    }
  }
}
