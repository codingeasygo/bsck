import { TestBed, inject } from '@angular/core/testing';

import { BsrouterService } from './bsrouter.service';

describe('BsrouterService', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [BsrouterService]
    });
  });

  it('should be created', inject([BsrouterService], (service: BsrouterService) => {
    expect(service).toBeTruthy();
  }));
});
