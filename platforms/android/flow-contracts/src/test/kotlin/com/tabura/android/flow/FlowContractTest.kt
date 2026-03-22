package com.tabura.android.flow

import java.io.InputStreamReader
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

class FlowContractTest {
    @Test
    fun sharedFlowsExecuteOnAndroidContract() {
        val bundle = loadFixtureBundle()
        assertEquals("android", bundle.platform)
        assertFalse(bundle.flows.isEmpty())
        bundle.flows.forEach { flow ->
            FlowRunner(bundle.platform, bundle.selectors).run(flow)
            println("android PASS ${flow.name}")
        }
    }

    private fun loadFixtureBundle(): FlowFixtureBundle {
        val stream = javaClass.classLoader.getResourceAsStream("flow-fixtures.json")
            ?: error("missing flow fixture resource")
        return InputStreamReader(stream, Charsets.UTF_8).use { reader ->
            FlowFixtureLoader.load(reader.readText())
        }
    }
}
