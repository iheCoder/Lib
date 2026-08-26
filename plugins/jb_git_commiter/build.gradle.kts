plugins {
    java
    id("org.jetbrains.intellij.platform") version "2.18.1"
}

group = "com.ihewe"
version = "0.6.0"

repositories {
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

dependencies {
    intellijPlatform {
        // CI can omit -PlocalIdePath and resolve the declared GoLand release. Local development
        // should point at an installed IDE to avoid downloading another full distribution.
        val localIdePath = providers.gradleProperty("localIdePath").orNull
        if (localIdePath.isNullOrBlank()) {
            goland("2026.2")
        } else {
            local(localIdePath)
        }
        bundledPlugin("Git4Idea")
        bundledPlugin("org.jetbrains.plugins.go")
    }

    testImplementation("org.junit.jupiter:junit-jupiter:5.11.4")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher:1.11.4")
}

java {
    toolchain {
        languageVersion = JavaLanguageVersion.of(25)
    }
}

tasks {
    withType<JavaCompile>().configureEach {
        options.encoding = "UTF-8"
        options.compilerArgs.add("-Xlint:deprecation")
    }

    test {
        useJUnitPlatform()
    }

    patchPluginXml {
        sinceBuild.set("262")
    }
}

intellijPlatform {
    buildSearchableOptions = false
    pluginConfiguration {
        name = "AI Git Committer"
        version = project.version.toString()
        description = "Generates commit messages from selected changes using OpenAI, Anthropic, or DeepSeek."
        vendor {
            name = "ihewe"
        }
    }
}
