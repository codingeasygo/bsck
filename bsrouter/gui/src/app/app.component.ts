import { Component, ViewChild, ElementRef } from '@angular/core';
declare var ipcRenderer: any;

@Component({
  selector: 'app-root',
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.css']
})
export class AppComponent {
  public activated: string = "basic"
  constructor() {
  }
  ngOnInit() {
  }
  doNavSwitch(e: { key: string }) {
    this.activated = e.key
    console.log("nav switch to " + e.key)
  }
  clickClose(e) {
    ipcRenderer.send("hideConfigure", {})
  }
}
