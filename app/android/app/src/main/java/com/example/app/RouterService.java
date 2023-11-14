package com.example.app;

import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.os.Binder;
import android.os.IBinder;
import android.os.PowerManager;
import android.util.Log;
import androidx.annotation.Nullable;
import bsrouter.Bsrouter;

public class RouterService extends Service {

  public static RouterService shared;

  public class RouterBind extends Binder {
    RouterService getService() { return RouterService.this; }
  }

  public RouterBind bind = new RouterBind();
  private PowerManager.WakeLock wakeLock;

  public RouterService() { shared = this; }

  @Nullable
  @Override
  public IBinder onBind(Intent intent) {
    return bind;
  }

  @Override
  public int onStartCommand(Intent intent, int flags, int startId) {
    try {
      PowerManager mgr =
          (PowerManager)this.getSystemService(Context.POWER_SERVICE);
      wakeLock =
          mgr.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "bsrouter:Service");
      wakeLock.acquire();
      String message =
          Bsrouter.start(this.getApplicationInfo().dataDir + "/.bsrouter.json");
      if (message.length() > 0) {
        Log.e("RouterService", "start bsrouter with " + message);
      } else {
        Log.i("RouterService", "start bsrouter success");
      }
    } catch (Exception e) {
      e.printStackTrace();
    }
    return super.onStartCommand(intent, flags, startId);
  }

  @Override
  public void onDestroy() {
    try {
      if (wakeLock != null) {
        wakeLock.release();
      }
      String message = Bsrouter.stop();
      if (message.length() > 0) {
        Log.e("RouterService", "stop bsrouter with " + message);
      } else {
        Log.i("RouterService", "stop bsrouter success");
      }
    } catch (Exception e) {
      e.printStackTrace();
    }
    super.onDestroy();
  }
}
