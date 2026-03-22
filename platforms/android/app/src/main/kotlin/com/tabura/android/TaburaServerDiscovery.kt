package com.tabura.android

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo

class TaburaServerDiscovery(
    context: Context,
    private val onServersChanged: (List<TaburaDiscoveredServer>) -> Unit,
    private val onError: (String) -> Unit,
) {
    private val nsdManager = context.getSystemService(Context.NSD_SERVICE) as NsdManager
    private val discovered = linkedMapOf<String, TaburaDiscoveredServer>()
    private var discoveryListener: NsdManager.DiscoveryListener? = null

    fun start() {
        if (discoveryListener != null) {
            return
        }
        val listener = object : NsdManager.DiscoveryListener {
            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                onError("NSD discovery failed: $errorCode")
            }

            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {
                onError("NSD discovery stop failed: $errorCode")
            }

            override fun onDiscoveryStarted(serviceType: String) {
                publish()
            }

            override fun onDiscoveryStopped(serviceType: String) {
                publish()
            }

            override fun onServiceFound(serviceInfo: NsdServiceInfo) {
                if (serviceInfo.serviceType != "_tabura._tcp.") {
                    return
                }
                nsdManager.resolveService(serviceInfo, object : NsdManager.ResolveListener {
                    override fun onResolveFailed(info: NsdServiceInfo, errorCode: Int) {
                        onError("NSD resolve failed for ${info.serviceName}: $errorCode")
                    }

                    override fun onServiceResolved(info: NsdServiceInfo) {
                        val host = info.host?.hostAddress?.ifBlank { info.host?.hostName.orEmpty() }.orEmpty()
                        if (host.isBlank()) {
                            return
                        }
                        val cleanHost = host.removeSuffix(".")
                        val id = "${info.serviceName}-$cleanHost-${info.port}"
                        discovered[id] = TaburaDiscoveredServer(
                            id = id,
                            name = info.serviceName ?: cleanHost,
                            host = cleanHost,
                            port = info.port,
                        )
                        publish()
                    }
                })
            }

            override fun onServiceLost(serviceInfo: NsdServiceInfo) {
                val serviceName = serviceInfo.serviceName ?: return
                val keys = discovered.keys.filter { it.startsWith("$serviceName-") }
                for (key in keys) {
                    discovered.remove(key)
                }
                publish()
            }
        }
        discoveryListener = listener
        nsdManager.discoverServices("_tabura._tcp.", NsdManager.PROTOCOL_DNS_SD, listener)
    }

    fun stop() {
        val listener = discoveryListener ?: return
        runCatching { nsdManager.stopServiceDiscovery(listener) }
        discoveryListener = null
        discovered.clear()
        publish()
    }

    private fun publish() {
        onServersChanged(discovered.values.sortedBy { it.name.lowercase() })
    }
}
