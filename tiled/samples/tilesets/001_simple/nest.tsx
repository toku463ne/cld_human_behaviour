<?xml version="1.0" encoding="UTF-8"?>
<tileset version="1.10" tiledversion="1.12.2" name="nest" tilewidth="32" tileheight="32" tilecount="8" columns="8">
 <image source="nest.png" width="256" height="32"/>
 <tile id="0">
  <properties>
   <property name="enemy" value="brute"/>
   <property name="nest" type="bool" value="true"/>
   <property name="roam" type="float" value="120"/>
   <property name="spawn" value="enemy"/>
  </properties>
 </tile>
 <tile id="1">
  <properties>
   <property name="enemy" value="stray"/>
   <property name="nest" type="bool" value="true"/>
   <property name="roam" type="float" value="260"/>
   <property name="spawn" value="enemy"/>
  </properties>
 </tile>
 <tile id="2">
  <properties>
   <property name="enemy" value="flyer"/>
   <property name="nest" type="bool" value="true"/>
   <property name="roam" type="float" value="300"/>
   <property name="spawn" value="enemy"/>
  </properties>
 </tile>
 <tile id="3">
  <properties>
   <property name="enemy" value="lurker"/>
   <property name="nest" type="bool" value="true"/>
   <property name="roam" type="float" value="90"/>
   <property name="spawn" value="enemy"/>
  </properties>
 </tile>
 <tile id="4">
  <properties>
   <property name="human" value="hearth"/>
   <property name="nest" type="bool" value="true"/>
  </properties>
 </tile>
 <tile id="5">
  <properties>
   <property name="human" value="haven"/>
   <property name="nest" type="bool" value="true"/>
  </properties>
 </tile>
 <tile id="6">
  <properties>
   <property name="human" value="camp"/>
   <property name="nest" type="bool" value="true"/>
  </properties>
 </tile>
 <tile id="7">
  <properties>
   <property name="human" value="landing"/>
   <property name="nest" type="bool" value="true"/>
  </properties>
 </tile>
</tileset>
