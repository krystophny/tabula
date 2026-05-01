package com.slopshell.android

internal fun slopshellInkStrokeFromPoints(
    pointerType: String,
    points: List<SlopshellInkPoint>,
): SlopshellInkStroke? {
    val cleanedPoints = dedupeInkPoints(points)
    if (cleanedPoints.isEmpty()) {
        return null
    }
    return SlopshellInkStroke(
        pointerType = pointerType,
        width = cleanedPoints.maxOf { it.pressure.coerceAtLeast(1f) } * INK_WIDTH_SCALE,
        points = cleanedPoints,
    )
}

private fun dedupeInkPoints(points: List<SlopshellInkPoint>): List<SlopshellInkPoint> {
    val seen = HashSet<InkPointKey>(points.size)
    return points.filter { point ->
        seen.add(InkPointKey(point.x, point.y, point.timestampMs))
    }
}

private data class InkPointKey(
    val x: Float,
    val y: Float,
    val timestampMs: Long,
)

private const val INK_WIDTH_SCALE = 2.4f
