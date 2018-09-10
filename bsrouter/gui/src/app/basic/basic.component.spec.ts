import { async, ComponentFixture, TestBed } from '@angular/core/testing';
import { BasicComponent } from './basic.component';
import { FormsModule } from '@angular/forms';
import { BrowserModule } from '@angular/platform-browser';
import { NgSelectModule } from '@ng-select/ng-select';
import { AngularDraggableModule } from 'angular2-draggable';
import { MockIpcRenderer, sleep } from '../bsrouter.testdata';
declare var global: any;

describe('BasicComponent', () => {
  let component: BasicComponent;
  let fixture: ComponentFixture<BasicComponent>;
  global.ipcRenderer = new MockIpcRenderer()
  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [BasicComponent],
      imports: [
        FormsModule,
        BrowserModule,
        NgSelectModule,
        AngularDraggableModule
      ],
    }).compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(BasicComponent);
    component = fixture.componentInstance;
    component.dimissDelay = 100;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('should save success', async () => {
    global.ipcRenderer.fail = false
    fixture.debugElement.nativeElement.querySelector(".app-basic-save .btn-info").click()
    await sleep(150)
  });

  it('should save fail', async () => {
    global.ipcRenderer.fail = true
    fixture.debugElement.nativeElement.querySelector(".app-basic-save .btn-info").click()
    await sleep(150)
  });
});
