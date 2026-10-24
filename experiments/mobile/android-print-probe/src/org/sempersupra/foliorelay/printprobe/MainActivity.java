package org.sempersupra.foliorelay.printprobe;

import android.app.Activity;
import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.os.Bundle;
import android.os.CancellationSignal;
import android.os.ParcelFileDescriptor;
import android.print.PageRange;
import android.print.PrintAttributes;
import android.print.PrintDocumentAdapter;
import android.print.PrintDocumentInfo;
import android.print.PrintManager;
import android.print.pdf.PrintedPdfDocument;
import android.view.WindowManager;

import java.io.FileOutputStream;
import java.io.IOException;

public final class MainActivity extends Activity {
    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);

        PrintManager manager = (PrintManager) getSystemService(Context.PRINT_SERVICE);
        PrintAttributes attributes = new PrintAttributes.Builder()
                .setMediaSize(PrintAttributes.MediaSize.ISO_A4)
                .setColorMode(PrintAttributes.COLOR_MODE_COLOR)
                .build();
        manager.print(
                "FolioRelay Android native probe",
                new ProbeDocumentAdapter(this),
                attributes);
    }

    private static final class ProbeDocumentAdapter extends PrintDocumentAdapter {
        private final Context context;
        private PrintAttributes attributes;

        ProbeDocumentAdapter(Context context) {
            this.context = context;
        }

        @Override
        public void onLayout(
                PrintAttributes oldAttributes,
                PrintAttributes newAttributes,
                CancellationSignal cancellationSignal,
                LayoutResultCallback callback,
                Bundle extras) {
            if (cancellationSignal.isCanceled()) {
                callback.onLayoutCancelled();
                return;
            }
            attributes = newAttributes;
            PrintDocumentInfo info = new PrintDocumentInfo.Builder("foliorelay-android-probe.pdf")
                    .setContentType(PrintDocumentInfo.CONTENT_TYPE_DOCUMENT)
                    .setPageCount(1)
                    .build();
            callback.onLayoutFinished(info, !newAttributes.equals(oldAttributes));
        }

        @Override
        public void onWrite(
                PageRange[] pages,
                ParcelFileDescriptor destination,
                CancellationSignal cancellationSignal,
                WriteResultCallback callback) {
            if (cancellationSignal.isCanceled()) {
                callback.onWriteCancelled();
                return;
            }

            PrintedPdfDocument document = new PrintedPdfDocument(context, attributes);
            try {
                android.graphics.pdf.PdfDocument.Page page = document.startPage(0);
                Canvas canvas = page.getCanvas();
                canvas.drawColor(Color.WHITE);
                Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
                paint.setColor(Color.BLACK);
                paint.setTextSize(22f);
                canvas.drawText("FolioRelay Android native PrintManager probe", 72f, 120f, paint);
                paint.setTextSize(14f);
                canvas.drawText("PrintManager -> Print Spooler -> BuiltInPrintService", 72f, 160f, paint);
                document.finishPage(page);

                try (FileOutputStream output =
                             new FileOutputStream(destination.getFileDescriptor())) {
                    document.writeTo(output);
                }
                callback.onWriteFinished(new PageRange[]{PageRange.ALL_PAGES});
            } catch (IOException error) {
                callback.onWriteFailed(error.toString());
            } finally {
                document.close();
            }
        }
    }
}
