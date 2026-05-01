package com.slopshell.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SlopshellInkStrokeBuilderTest {
    @Test
    fun strokeBuilderDropsDuplicateSamplesAndScalesWidth() {
        val stroke = slopshellInkStrokeFromPoints(
            pointerType = "stylus",
            points = listOf(
                point(x = 1f, y = 2f, pressure = 0.25f, timestampMs = 10L),
                point(x = 1f, y = 2f, pressure = 0.9f, timestampMs = 10L),
                point(x = 3f, y = 4f, pressure = 1.5f, timestampMs = 11L),
            ),
        ) ?: error("expected stroke")

        assertEquals("stylus", stroke.pointerType)
        assertEquals(2, stroke.points.size)
        assertEquals(3.6f, stroke.width, 0.0001f)
        assertEquals(0.25f, stroke.points[0].pressure, 0.0001f)
        assertEquals(1.5f, stroke.points[1].pressure, 0.0001f)
    }

    @Test
    fun strokeBuilderRejectsEmptySamples() {
        assertNull(slopshellInkStrokeFromPoints(pointerType = "touch", points = emptyList()))
    }

    private fun point(
        x: Float,
        y: Float,
        pressure: Float,
        timestampMs: Long,
    ): SlopshellInkPoint {
        return SlopshellInkPoint(
            x = x,
            y = y,
            pressure = pressure,
            tiltX = 0f,
            tiltY = 0f,
            roll = 0f,
            timestampMs = timestampMs,
        )
    }
}
