package com.tabura.android

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.media.AudioFormat
import android.media.AudioRecord
import android.media.MediaRecorder
import android.os.Binder
import android.os.IBinder
import android.os.PowerManager
import android.os.Process
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlin.math.abs

class TaburaAudioCaptureService : Service() {
    interface Listener {
        fun onAudioChunk(data: ByteArray)
        fun onRecordingStateChanged(active: Boolean)
        fun onAudioError(message: String)
    }

    inner class LocalBinder : Binder() {
        fun service(): TaburaAudioCaptureService = this@TaburaAudioCaptureService
    }

    private val binder = LocalBinder()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var listener: Listener? = null
    private var captureJob: Job? = null
    private var audioRecord: AudioRecord? = null
    private var wakeLock: PowerManager.WakeLock? = null

    override fun onBind(intent: Intent?): IBinder {
        return binder
    }

    override fun onCreate() {
        super.onCreate()
        ensureNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        return START_STICKY
    }

    override fun onDestroy() {
        stopStreaming()
        scope.cancel()
        super.onDestroy()
    }

    fun setListener(listener: Listener?) {
        this.listener = listener
    }

    fun isRunning(): Boolean {
        return captureJob?.isActive == true
    }

    fun startStreaming() {
        if (isRunning()) {
            return
        }
        val bufferSize = AudioRecord.getMinBufferSize(
            SAMPLE_RATE,
            AudioFormat.CHANNEL_IN_MONO,
            AudioFormat.ENCODING_PCM_16BIT,
        ).coerceAtLeast(4096)
        val record = AudioRecord(
            MediaRecorder.AudioSource.VOICE_RECOGNITION,
            SAMPLE_RATE,
            AudioFormat.CHANNEL_IN_MONO,
            AudioFormat.ENCODING_PCM_16BIT,
            bufferSize,
        )
        if (record.state != AudioRecord.STATE_INITIALIZED) {
            record.release()
            listener?.onAudioError("AudioRecord initialization failed")
            return
        }
        audioRecord = record
        acquireWakeLock()
        startForeground(NOTIFICATION_ID, buildNotification())
        record.startRecording()
        listener?.onRecordingStateChanged(true)
        captureJob = scope.launch(Dispatchers.IO) {
            Process.setThreadPriority(Process.THREAD_PRIORITY_AUDIO)
            captureLoop(record, bufferSize)
        }
    }

    fun stopStreaming() {
        captureJob?.cancel()
        captureJob = null
        runCatching {
            audioRecord?.stop()
        }
        audioRecord?.release()
        audioRecord = null
        releaseWakeLock()
        stopForeground(STOP_FOREGROUND_REMOVE)
        listener?.onRecordingStateChanged(false)
    }

    private suspend fun captureLoop(record: AudioRecord, bufferSize: Int) {
        val buffer = ByteArray(bufferSize)
        while (currentCoroutineContext().isActive) {
            val read = record.read(buffer, 0, buffer.size)
            if (read <= 0) {
                continue
            }
            val chunk = buffer.copyOf(read)
            if (voiceDetected(chunk)) {
                listener?.onAudioChunk(chunk)
            }
        }
    }

    private fun voiceDetected(chunk: ByteArray): Boolean {
        if (chunk.size < 2) {
            return false
        }
        var total = 0.0
        var sampleCount = 0
        var index = 0
        while (index + 1 < chunk.size) {
            val sample = ((chunk[index + 1].toInt() shl 8) or (chunk[index].toInt() and 0xff)).toShort()
            total += abs(sample.toInt().toDouble())
            sampleCount += 1
            index += 2
        }
        if (sampleCount == 0) {
            return false
        }
        val average = total / sampleCount
        return average >= VOICE_THRESHOLD
    }

    private fun acquireWakeLock() {
        if (wakeLock?.isHeld == true) {
            return
        }
        val powerManager = getSystemService(POWER_SERVICE) as PowerManager
        wakeLock = powerManager.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "tabura:audio")
        wakeLock?.acquire()
    }

    private fun releaseWakeLock() {
        wakeLock?.takeIf { it.isHeld }?.release()
        wakeLock = null
    }

    private fun buildNotification(): Notification {
        val intent = Intent(this, MainActivity::class.java)
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.audio_notification_title))
            .setContentText(getString(R.string.audio_notification_text))
            .setSmallIcon(android.R.drawable.ic_btn_speak_now)
            .setOngoing(true)
            .setContentIntent(pendingIntent)
            .build()
    }

    private fun ensureNotificationChannel() {
        val manager = getSystemService(NotificationManager::class.java)
        if (manager.getNotificationChannel(CHANNEL_ID) != null) {
            return
        }
        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_ID,
                getString(R.string.audio_notification_channel),
                NotificationManager.IMPORTANCE_LOW,
            )
        )
    }

    companion object {
        private const val CHANNEL_ID = "tabura-audio"
        private const val NOTIFICATION_ID = 8420
        private const val SAMPLE_RATE = 16_000
        private const val VOICE_THRESHOLD = 900.0
    }
}
