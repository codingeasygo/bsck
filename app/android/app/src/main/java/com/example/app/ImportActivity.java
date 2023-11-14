package com.example.app;

import androidx.appcompat.app.AppCompatActivity;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.widget.TextView;
import android.widget.Toast;

import java.io.File;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.StringWriter;

public class ImportActivity extends Activity {

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_import);
        handleIntent();
    }

    private void handleIntent() {

        TextView textView = findViewById(R.id.message);
        Uri uri = getIntent().getData();
        if (uri == null) {
            Toast.makeText(this, "import fail", Toast.LENGTH_SHORT).show();
            textView.setText("import fail");
            return;
        }
        String oldname = getApplicationInfo().dataDir + "/.bsrouter.bak.json";
        String filename = getApplicationInfo().dataDir + "/.bsrouter.json";
        new File(oldname).delete();
        boolean old = new File(filename).renameTo(new File(oldname));
        InputStream input = null;
        FileOutputStream output = null;
        try {
            input = getContentResolver().openInputStream(uri);
            output = new FileOutputStream(filename);
            byte[] buffer = new byte[4 * 1024];
            while (true) {
                int n = input.read(buffer);
                if (n < 0) {
                    break;
                }
                output.write(buffer, 0, n);
            }
            input.close();
            output.close();
            Toast.makeText(this, "import success", Toast.LENGTH_SHORT).show();
            startActivity(new Intent(this, MainActivity.class));
            finish();
        } catch (Exception e) {
            e.printStackTrace();
            if (old) {
                new File(oldname).renameTo(new File(filename));
            }
            try {
                if (input != null) {
                    input.close();
                    ;
                }
            } catch (Exception e1) {
            }
            try {
                if (output != null) {
                    output.close();
                }
            } catch (Exception e2) {
            }
            Toast.makeText(this, "import fail", Toast.LENGTH_SHORT).show();
            textView.setText(e.getMessage());
        }
    }
}