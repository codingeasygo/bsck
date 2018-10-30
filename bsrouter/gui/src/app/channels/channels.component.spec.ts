import { async, ComponentFixture, TestBed } from '@angular/core/testing';
import { ChannelsComponent } from './channels.component';
import { FormsModule } from '@angular/forms';
import { BrowserModule, By } from '@angular/platform-browser';
import { NgSelectModule } from '@ng-select/ng-select';
import { AngularDraggableModule } from 'angular2-draggable';
import { MockIpcRenderer } from '../bsrouter.testdata';
declare var global: any;

describe('ChannelsComponent', () => {
  let component: ChannelsComponent;
  let fixture: ComponentFixture<ChannelsComponent>;
  global.ipcRenderer = new MockIpcRenderer()
  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ChannelsComponent],
      imports: [
        FormsModule,
        BrowserModule,
        NgSelectModule,
        AngularDraggableModule
      ],
    }).compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(ChannelsComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('should add fail', () => {
    component.add()
    expect(component.showError).toBeTruthy()
    component.channel.remote = "x"
    component.add()
    expect(component.showError).toBeTruthy()
    component.channel.token = "x"
    delete component.channel.index
    component.add()
    expect(component.showError).toBeTruthy()
    component.channel.index = -1
    component.add()
    expect(component.showError).toBeTruthy()
  });

  it('should add success', () => {
    component.channel.remote = "x"
    component.channel.token = "x"
    component.channel.index = 0
    component.add()
    expect(component.showError).toBeFalsy()
    component.remove(component.allChannels.length - 1)
  });

  it('should enbable success', () => {
    let cbs = fixture.debugElement.queryAll(By.css("input[type='checkbox']"));
    cbs[0].nativeElement.click();
    fixture.detectChanges();
  });
});
