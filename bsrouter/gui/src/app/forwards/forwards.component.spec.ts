import { async, ComponentFixture, TestBed } from '@angular/core/testing';
import { ForwardsComponent } from './forwards.component';
import { FormsModule } from '@angular/forms';
import { BrowserModule } from '@angular/platform-browser';
import { NgSelectModule } from '@ng-select/ng-select';
import { AngularDraggableModule } from 'angular2-draggable';
import { MockIpcRenderer, URL } from '../bsrouter.testdata';
declare var global: any;

describe('ForwardsComponent', () => {
  let component: ForwardsComponent;
  let fixture: ComponentFixture<ForwardsComponent>;
  global.ipcRenderer = new MockIpcRenderer()
  global.url = URL
  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ForwardsComponent],
      imports: [
        FormsModule,
        BrowserModule,
        NgSelectModule,
        AngularDraggableModule
      ],
    })
      .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(ForwardsComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('should add false', () => {
    component.forward.protocol = ""
    component.add()
    expect(component.showError).toBeTruthy()
    component.forward.protocol = "ws"
    component.add()
    expect(component.showError).toBeTruthy()
    component.forward.name = "xx"
    component.add()
    expect(component.showError).toBeTruthy()
  });

  it('should add success', () => {
    component.forward.protocol = "ws"
    component.forward.name = "xx1"
    component.forward.router = "xx"
    component.add()
    expect(component.showError).toBeFalsy()
    //
    component.forward.protocol = "ws"
    component.forward.name = "xx2"
    component.forward.router = "xx"
    component.forward.username = "u1"
    component.forward.password = "u1"
    component.forward.address = "localhost:80"
    component.add()
    expect(component.showError).toBeFalsy()
    //
    component.forward.protocol = "tcp"
    component.forward.name = "xx2"
    component.forward.router = "xx"
    component.forward.username = "u1"
    component.forward.password = "u1"
    component.add()
    expect(component.showError).toBeFalsy()
    //
    component.remove("xx1~ws://")
  });

  it('should open forward', () => {
    component.open({ k: "xx1~ws://" })
    component.open({ k: "xx" })
    component.open({ k: "error" })
  });

});
