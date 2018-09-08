import { Component, OnInit, Input } from '@angular/core';
import { BsrouterService } from '../bsrouter.service';

@Component({
  selector: 'app-channels',
  templateUrl: './channels.component.html',
  styleUrls: ['./channels.component.css']
})
export class ChannelsComponent implements OnInit {
  srv: BsrouterService;
  allChannels: any = []
  channel: any = { index: 0 }
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
    this.allChannels = this.srv.loadChannels();
  }
  remove(i: number) {
    try {
      this.srv.removeChannel(i);
      this.reload();
    } catch (e) {
      window.alert(e)
    }
  }
  add() {
    try {
      if (!this.channel.remote || !this.channel.token || this.channel.index == undefined || this.channel.index < 0) {
        this.showError = true
        return;
      }
      this.showError = false
      this.srv.addChannel(this.channel)
      this.channel = { index: 0 }
      this.reload()
    } catch (e) {
      window.alert(e)
    }
  }
}
