package com.example.app;


import android.content.Intent;

import androidx.annotation.NonNull;

import bsrouter.Bsrouter;
import io.flutter.embedding.android.FlutterActivity;
import io.flutter.embedding.engine.FlutterEngine;
import io.flutter.plugin.common.MethodCall;
import io.flutter.plugin.common.MethodChannel;

public class MainActivity extends FlutterActivity {

    @Override
    public void configureFlutterEngine(@NonNull FlutterEngine flutterEngine) {
        super.configureFlutterEngine(flutterEngine);
        new MethodChannel(flutterEngine.getDartExecutor().getBinaryMessenger(), "bsrouter").setMethodCallHandler(bsrouter );
    }

    MethodChannel.MethodCallHandler bsrouter=new MethodChannel.MethodCallHandler(){

        @Override
        public void onMethodCall(@NonNull MethodCall call, @NonNull MethodChannel.Result result) {
            Intent intent=new Intent(MainActivity.this,RouterService.class);
            switch (call.method){
                case "state":
                    result.success(Bsrouter.state(MainActivity.this.getApplicationInfo().dataDir + "/.bsrouter.json"));
                    break;
                case "start":
                    MainActivity.this.startService(intent);
                    break;
                case "stop":
                    MainActivity.this.stopService(intent);
                    break;
            }
        }
    };

//    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
//        super.configureFlutterEngine(flutterEngine)
//        MethodChannel(flutterEngine.dartExecutor.binaryMessenger,"bsrouter").setMethodCallHandler(bsrouter);
//    }

//    var bsrouter= MethodChannel.MethodCallHandler() { methodCall: MethodCall, result: MethodChannel.Result ->
//        if(methodCall.method=="start"){
//            val intent = Intent(context, RouterService::class.java)
//            context?.startService(intent)
//            result.success("ok");
//        }else if(methodCall.method=="stop"){
//            val intent = Intent(context, RouterService::class.java)
//            context?.stopService(intent);
//            result.success("ok");
//        }else if (methodCall.method=="state"){
//            result.success(Bsrouter.state())
//        }
//    }
}
