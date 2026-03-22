package com.tabura.android

import android.content.Context
import android.graphics.Color
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView

@Composable
fun TaburaCanvasWebView(
    html: String,
    baseUrl: String,
    isEinkDisplay: Boolean = false,
    modifier: Modifier = Modifier,
) {
    val renderedHtml = if (isEinkDisplay) {
        applyEinkDisplayHtml(html)
    } else {
        html
    }
    AndroidView(
        modifier = modifier,
        factory = { context ->
            TaburaCanvasDisplayWebView(context).apply {
                setBackgroundColor(Color.TRANSPARENT)
                settings.javaScriptEnabled = false
                settings.allowFileAccess = false
                settings.allowContentAccess = false
                settings.domStorageEnabled = false
                webViewClient = object : WebViewClient() {
                    override fun onPageFinished(view: WebView, url: String?) {
                        super.onPageFinished(view, url)
                        (view as? TaburaCanvasDisplayWebView)?.onContentRendered()
                    }
                }
            }
        },
        update = { view ->
            view.setEinkDisplay(isEinkDisplay)
            view.loadDataWithBaseURL(baseUrl, renderedHtml, "text/html", "utf-8", null)
        },
    )
}

private class TaburaCanvasDisplayWebView(
    context: Context,
) : WebView(context) {
    private var isEinkDisplay = false
    private var pendingRefresh: Runnable? = null

    fun setEinkDisplay(enabled: Boolean) {
        isEinkDisplay = enabled
        if (enabled) {
            TaburaBooxEinkController.configureContentView(this)
            TaburaBooxEinkController.setWebViewContrastOptimize(this, true)
            return
        }
        pendingRefresh?.let(::removeCallbacks)
        pendingRefresh = null
        TaburaBooxEinkController.setWebViewContrastOptimize(this, false)
    }

    fun onContentRendered() {
        if (!isEinkDisplay) {
            return
        }
        TaburaBooxEinkController.configureContentView(this)
        scheduleRefresh()
    }

    override fun onDetachedFromWindow() {
        pendingRefresh?.let(::removeCallbacks)
        pendingRefresh = null
        super.onDetachedFromWindow()
    }

    private fun scheduleRefresh() {
        pendingRefresh?.let(::removeCallbacks)
        val refresh = Runnable {
            TaburaBooxEinkController.refreshContentView(this)
        }
        pendingRefresh = refresh
        postDelayed(refresh, 160L)
    }
}

private fun applyEinkDisplayHtml(html: String): String {
    val withBodyClass = when {
        BODY_TAG.containsMatchIn(html) -> BODY_TAG.replaceFirst(html) { match ->
            val attributes = match.groupValues[1]
            if (BODY_CLASS_TAG.containsMatchIn(attributes)) {
                "<body${BODY_CLASS_TAG.replaceFirst(attributes) { classMatch ->
                    val classes = classMatch.groupValues[1]
                        .split(Regex("\\s+"))
                        .filter { it.isNotBlank() }
                        .toMutableList()
                    if (!classes.contains("eink-display")) {
                        classes += "eink-display"
                    }
                    " class=\"${classes.joinToString(" ")}\""
                }}>"
            } else {
                "<body$attributes class=\"eink-display\">"
            }
        }
        else -> "<html><body class=\"eink-display\">$html</body></html>"
    }
    return if (HEAD_END_TAG.containsMatchIn(withBodyClass)) {
        HEAD_END_TAG.replaceFirst(withBodyClass, "$EINK_STYLE</head>")
    } else {
        BODY_TAG.replaceFirst(withBodyClass, "<head>$EINK_STYLE</head>$0")
    }
}

private val BODY_TAG = Regex("<body([^>]*)>", RegexOption.IGNORE_CASE)
private val BODY_CLASS_TAG = Regex("class\\s*=\\s*\"([^\"]*)\"", RegexOption.IGNORE_CASE)
private val HEAD_END_TAG = Regex("</head>", RegexOption.IGNORE_CASE)
private val EINK_STYLE = """
<style>
html, body {
  background: #fff !important;
  color: #000 !important;
}
body.eink-display,
body.eink-display * {
  transition: none !important;
  animation: none !important;
  background-image: none !important;
  box-shadow: none !important;
  text-shadow: none !important;
  filter: none !important;
  scroll-behavior: auto !important;
}
body.eink-display a,
body.eink-display pre,
body.eink-display code,
body.eink-display table,
body.eink-display th,
body.eink-display td,
body.eink-display blockquote,
body.eink-display hr {
  color: #000 !important;
  border-color: #000 !important;
}
body.eink-display [style*="gradient"],
body.eink-display [style*="opacity"],
body.eink-display [style*="shadow"] {
  background: #fff !important;
  opacity: 1 !important;
}
</style>
""".trimIndent()
