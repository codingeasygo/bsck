import { TestBed, inject } from '@angular/core/testing';
import { BsrouterService } from './bsrouter.service';
import { MockIpcRenderer } from './bsrouter.testdata';
declare var global: any;

describe('BsrouterService', () => {
  beforeEach(() => {
    global.ipcRenderer = new MockIpcRenderer()
    TestBed.configureTestingModule({
      providers: [BsrouterService]
    });
  });

  it('should be created', inject([BsrouterService], (service: BsrouterService) => {
    expect(service).toBeTruthy();
  }));
});
